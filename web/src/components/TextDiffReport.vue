<script setup lang="ts">
import { computed, reactive, ref } from "vue";
import { useAuthStore } from "../stores/auth";
import { useTextDiffStore } from "../stores/textDiff";
import { useToastStore } from "../stores/toast";
import type {
	TextDiff,
	TextDiffDirection,
	TextDiffHunk,
	TextDiffLine,
	TextDiffPage,
} from "../types";

/**
 * One job's update text diff: per page, its outcome and the changed lines
 * with context. A removed line followed by an added one is shown with the
 * changed part highlighted; any changed line can be muted for the site.
 */
const props = defineProps<{
	diff: TextDiff;
	siteId: string;
	domain?: string;
}>();

const authStore = useAuthStore();
const textDiffStore = useTextDiffStore();
const toast = useToastStore();

const OUTCOMES: Record<string, { label: string; badge: string }> = {
	unchanged: { label: "Unchanged", badge: "success" },
	changed: { label: "Changed", badge: "warning" },
	error: { label: "Broken", badge: "error" },
	no_baseline: { label: "Not checked", badge: "" },
};

/** Pages with something to show first, unchanged ones last. */
const pages = computed(() => {
	const order = ["error", "changed", "no_baseline", "unchanged"];
	return [...props.diff.pages].sort(
		(a, b) =>
			order.indexOf(a.outcome ?? "") - order.indexOf(b.outcome ?? ""),
	);
});

function liveUrl(path: string) {
	return props.domain ? `https://${props.domain}${path}` : undefined;
}

/** A line split into the part shared with its counterpart and the change. */
interface Segment {
	text: string;
	changed: boolean;
}

/**
 * Splits each line of a hunk into segments. A run of removed lines
 * followed by as many added ones pairs them up, and each pair highlights
 * only what differs between the two (common prefix and suffix are plain).
 */
const segmentCache = new WeakMap<TextDiffHunk, Segment[][]>();

function segmentsFor(hunk: TextDiffHunk): Segment[][] {
	let segs = segmentCache.get(hunk);
	if (!segs) {
		segs = segments(hunk);
		segmentCache.set(hunk, segs);
	}
	return segs;
}

function segments(hunk: TextDiffHunk): Segment[][] {
	const out: Segment[][] = hunk.lines.map((l) => [
		{ text: l.text, changed: l.kind !== "context" },
	]);
	const lines = hunk.lines;
	for (let i = 0; i < lines.length; ) {
		if (lines[i]!.kind !== "removed") {
			i++;
			continue;
		}
		let r = i;
		while (r < lines.length && lines[r]!.kind === "removed") r++;
		let a = r;
		while (a < lines.length && lines[a]!.kind === "added") a++;
		const pairs = Math.min(r - i, a - r);
		for (let k = 0; k < pairs; k++) {
			const [x, y] = highlight(lines[i + k]!.text, lines[r + k]!.text);
			out[i + k] = x;
			out[r + k] = y;
		}
		i = a;
	}
	return out;
}

function highlight(a: string, b: string): [Segment[], Segment[]] {
	let pre = 0;
	while (pre < a.length && pre < b.length && a[pre] === b[pre]) pre++;
	let suf = 0;
	while (
		suf < a.length - pre &&
		suf < b.length - pre &&
		a[a.length - 1 - suf] === b[b.length - 1 - suf]
	) {
		suf++;
	}
	const split = (s: string): Segment[] =>
		[
			{ text: s.slice(0, pre), changed: false },
			{ text: s.slice(pre, s.length - suf), changed: true },
			{ text: s.slice(s.length - suf), changed: false },
		].filter((seg) => seg.text !== "");
	return [split(a), split(b)];
}

// --- Muting ---

const lineKey = (path: string, l: TextDiffLine) =>
	`${path}\u0000${l.kind}\u0000${l.text}`;

/** Lines muted from this report. */
const muted = ref(new Set<string>());
/** The line whose mute form is open. */
const muting = ref<string | null>(null);
const form = reactive({ reason: "", days: 0 });
const saving = ref(false);

function openMute(page: TextDiffPage, l: TextDiffLine) {
	muting.value = lineKey(page.path, l);
	form.reason = "";
	form.days = 0;
}

async function saveMute(page: TextDiffPage, l: TextDiffLine) {
	saving.value = true;
	try {
		await textDiffStore.createMute(props.siteId, {
			path: page.path,
			direction: l.kind as TextDiffDirection,
			text: l.text,
			reason: form.reason.trim() || undefined,
			days: form.days || undefined,
		});
		muted.value = new Set(muted.value).add(lineKey(page.path, l));
		muting.value = null;
		toast.addToast(
			l.kind === "added"
				? "Muted: this line appearing won't be reported again on this page."
				: "Muted: this line disappearing won't be reported again on this page.",
			"success",
		);
	} catch (e: any) {
		toast.addToast(e.message || "Failed to mute the line", "error");
	} finally {
		saving.value = false;
	}
}
</script>

<template>
	<div class="text-diff">
		<p v-if="diff.status === 'running'" class="text-muted">
			Checking the site's pages…
		</p>
		<p v-else-if="diff.status === 'interrupted'" class="text-muted">
			The check was interrupted by a jman-api restart.
		</p>

		<section v-for="page in pages" :key="page.path" class="text-diff-page">
			<header class="text-diff-page-header">
				<code class="text-diff-path">{{ page.path }}</code>
				<span
					v-if="page.outcome"
					:class="[
						'status-badge',
						'badge-sm',
						OUTCOMES[page.outcome]?.badge,
					]"
					>{{ OUTCOMES[page.outcome]?.label ?? page.outcome }}</span
				>
				<span v-if="page.hidden" class="sub-text"
					>{{ page.hidden }} noisy or muted line{{
						page.hidden === 1 ? "" : "s"
					}}
					hidden</span
				>
				<a
					v-if="liveUrl(page.path)"
					:href="liveUrl(page.path)"
					target="_blank"
					rel="noopener noreferrer"
					class="text-diff-live"
					>Open page ↗</a
				>
			</header>

			<p v-if="page.outcome === 'error'" class="form-error">
				Worked before the update, fails now: {{ page.error }}
			</p>
			<p v-else-if="page.outcome === 'no_baseline'" class="text-muted">
				Couldn't be fetched before the update ({{ page.error }}), so
				there is nothing to compare.
			</p>
			<p v-if="page.final_url_after" class="text-diff-redirect">
				Now ends up at <code>{{ page.final_url_after }}</code> instead
				of <code>{{ page.final_url_before }}</code
				>.
			</p>

			<div
				v-for="(hunk, h) in page.hunks ?? []"
				:key="h"
				class="text-diff-hunk"
			>
				<template v-for="(l, i) in hunk.lines" :key="`${h}-${i}`">
					<div
						:class="[
							'text-diff-line',
							`text-diff-line--${l.kind}`,
							{
								'text-diff-line--muted': muted.has(
									lineKey(page.path, l),
								),
							},
						]"
					>
						<span class="text-diff-sign" aria-hidden="true">{{
							l.kind === "added"
								? "+"
								: l.kind === "removed"
									? "−"
									: " "
						}}</span>
						<span class="sr-only">{{
							l.kind === "context" ? "" : `${l.kind}: `
						}}</span>
						<span class="text-diff-text"
							><template
								v-for="(seg, s) in segmentsFor(hunk)[i]"
								:key="s"
								><mark
									v-if="seg.changed && l.kind !== 'context'"
									>{{ seg.text }}</mark
								><template v-else>{{
									seg.text
								}}</template></template
							></span
						>
						<span
							v-if="muted.has(lineKey(page.path, l))"
							class="sub-text"
							>Muted</span
						>
						<button
							v-else-if="
								l.kind !== 'context' && authStore.canExecute
							"
							class="btn btn-outline btn-sm text-diff-mute"
							@click="openMute(page, l)"
						>
							Mute
						</button>
					</div>
					<form
						v-if="muting === lineKey(page.path, l)"
						class="text-diff-mute-form"
						@submit.prevent="saveMute(page, l)"
					>
						<p class="sub-text">
							Stop reporting this line
							{{
								l.kind === "added"
									? "appearing"
									: "disappearing"
							}}
							on <code>{{ page.path }}</code> of this site.
							Numbers and dates in it may change.
						</p>
						<input
							v-model="form.reason"
							type="text"
							maxlength="500"
							placeholder="Reason (optional)"
							aria-label="Reason"
						/>
						<select v-model.number="form.days" aria-label="For">
							<option :value="0">Default time</option>
							<option :value="7">7 days</option>
							<option :value="30">30 days</option>
							<option :value="90">90 days</option>
							<option :value="365">1 year</option>
						</select>
						<button
							type="submit"
							class="btn btn-primary btn-sm"
							:disabled="saving"
						>
							Mute
						</button>
						<button
							type="button"
							class="btn btn-outline btn-sm"
							@click="muting = null"
						>
							Cancel
						</button>
					</form>
				</template>
			</div>
		</section>
	</div>
</template>

<style scoped>
.text-diff {
	display: flex;
	flex-direction: column;
	gap: var(--space-4);
}

.text-diff-page-header {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: var(--space-2);
	margin-bottom: var(--space-2);
}

.text-diff-path {
	font-weight: 600;
}

.text-diff-live {
	margin-left: auto;
	font-size: 13px;
}

.text-diff-redirect {
	font-size: 13px;
	margin: 0 0 var(--space-2);
	overflow-wrap: anywhere;
}

.text-diff-hunk {
	border: 1px solid var(--border-color);
	border-radius: 6px;
	overflow: hidden;
	font-size: 13px;
}

.text-diff-hunk + .text-diff-hunk {
	margin-top: var(--space-2);
}

.text-diff-line {
	display: flex;
	align-items: flex-start;
	gap: var(--space-2);
	padding: 2px var(--space-2);
	min-height: 26px;
}

.text-diff-line--context {
	color: var(--text-muted);
}

.text-diff-line--removed {
	background: var(--error-bg);
	color: var(--error-text);
}

.text-diff-line--added {
	background: var(--badge-active-bg);
	color: var(--badge-active-text);
}

.text-diff-line--muted {
	opacity: 0.55;
}

.text-diff-sign {
	flex: none;
	width: 1ch;
	font-family: ui-monospace, monospace;
}

.text-diff-text {
	flex: 1;
	min-width: 0;
	overflow-wrap: anywhere;
	white-space: pre-wrap;
}

.text-diff-text mark {
	/* A stronger tint of the line's own color, so it works in both themes. */
	background: color-mix(in srgb, currentColor 30%, transparent);
	color: inherit;
	font-weight: 700;
	border-radius: 2px;
	padding: 0 1px;
}

.text-diff-mute {
	flex: none;
	padding: 0 8px;
	font-size: 12px;
}

.text-diff-mute-form {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: var(--space-2);
	padding: var(--space-2);
	background: var(--bg-hover);
	border-top: 1px solid var(--border-color);
	border-bottom: 1px solid var(--border-color);
}

.text-diff-mute-form p {
	flex-basis: 100%;
	margin: 0;
}

.text-diff-mute-form input {
	flex: 1;
	min-width: 160px;
}
</style>
