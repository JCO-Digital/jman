<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useScanStore } from "../../stores/scan";
import { useDataStore } from "../../stores/data";
import { useToastStore } from "../../stores/toast";
import AppIcon from "../AppIcon.vue";
import type { ScanFinding } from "../../types";
import {
	canAllowlist,
	pluginName,
	pluginSlug,
	unknownPluginRule,
} from "./scan";

interface Props {
	/**
	 * allowlist trusts the findings' hashes (one file's content, or the
	 * slugs of one or more plugins); acknowledge trusts the paths. Both
	 * offer to cover the other open findings that match.
	 */
	mode: "allowlist" | "acknowledge" | null;
	findings: ScanFinding[];
}

const props = defineProps<Props>();

const emit = defineEmits<{
	(e: "close"): void;
	(e: "done"): void;
}>();

const scanStore = useScanStore();
const dataStore = useDataStore();
const toastStore = useToastStore();

const reason = ref("");
const closeMatching = ref(false);
const matching = ref<ScanFinding[] | null>(null);
const saving = ref(false);

const finding = computed(() => props.findings[0]);
// Plugins are allowlisted by slug, several at once.
const isPlugins = computed(
	() =>
		props.findings.length > 0 &&
		props.findings.every((f) => f.rule === unknownPluginRule),
);

watch(
	() => props.mode,
	async (mode) => {
		reason.value = "";
		// A plugin is trusted everywhere once allowlisted, so its open
		// findings elsewhere are covered by default; files are opt-in.
		closeMatching.value = mode === "allowlist" && isPlugins.value;
		matching.value = null;
		if (!mode) return;
		const targets = props.findings;
		try {
			const found =
				mode === "allowlist"
					? await findSameHashes(targets)
					: await findSamePaths(targets);
			if (props.findings === targets) matching.value = found;
		} catch (e) {
			toastStore.addToast(
				e instanceof Error
					? e.message
					: "Failed to load matching findings",
				"error",
			);
		}
	},
);

/**
 * The other open findings with the same hashes (file content or plugin
 * slug) that an allowlist entry would cover.
 */
async function findSameHashes(targets: ScanFinding[]): Promise<ScanFinding[]> {
	const selected = new Set(targets.map((f) => f.id));
	const hashes = [...new Set(targets.map((f) => f.sha256).filter(Boolean))];
	const lists = await Promise.all(
		hashes.map((sha256) =>
			scanStore.fetchFindings({ sha256, status: "open", limit: 1000 }),
		),
	);
	return lists.flat().filter((f) => !selected.has(f.id) && canAllowlist(f));
}

/**
 * The other open findings of the same rule and path, on any site: the same
 * unknown plugin or stray file elsewhere.
 */
async function findSamePaths(targets: ScanFinding[]): Promise<ScanFinding[]> {
	const selected = new Set(targets.map((f) => f.id));
	const keys = new Map<string, ScanFinding>();
	for (const f of targets) keys.set(`${f.rule}\0${f.path}`, f);
	const lists = await Promise.all(
		[...keys.values()].map((f) =>
			scanStore.fetchFindings({
				rule: f.rule,
				path: f.path,
				status: "open",
				limit: 1000,
			}),
		),
	);
	const byID = new Map<number, ScanFinding>();
	for (const f of lists.flat()) {
		if (!selected.has(f.id)) byID.set(f.id, f);
	}
	return [...byID.values()];
}

/** The selected plugins, one row per slug with the sites it's on. */
const pluginList = computed(() => {
	const bySlug = new Map<
		string,
		{ slug: string; name: string; sites: string[] }
	>();
	for (const f of props.findings) {
		const slug = pluginSlug(f.path);
		const row = bySlug.get(slug) ?? {
			slug,
			name: pluginName(f.snippet),
			sites: [],
		};
		row.sites.push(siteDomain(f.site_id));
		bySlug.set(slug, row);
	}
	return [...bySlug.values()];
});

const matchingSites = computed(() => {
	const domains = new Set(
		(matching.value ?? []).map((f) => siteDomain(f.site_id)),
	);
	return [...domains].sort();
});

function siteDomain(siteId: string): string {
	return dataStore.sites.find((s) => s.id === siteId)?.domain ?? siteId;
}

async function submit() {
	const why = reason.value.trim();
	if (!why || !props.mode || !props.findings.length) return;
	saving.value = true;
	try {
		if (props.mode === "allowlist") {
			const res = await scanStore.addAllowlist(
				props.findings.map((f) => f.id),
				why,
				closeMatching.value,
			);
			const what = isPlugins.value
				? `${res?.entries.length ?? 0} plugin(s)`
				: "Hash";
			toastStore.addToast(
				`${what} allowlisted; ${res?.allowlisted ?? 0} finding(s) marked allowlisted`,
				"success",
			);
		} else {
			const ids = props.findings.map((f) => f.id);
			if (closeMatching.value && matching.value) {
				ids.push(...matching.value.map((f) => f.id));
			}
			let acknowledged = 0;
			// jman-api takes at most 500 findings per request.
			for (let i = 0; i < ids.length; i += 500) {
				const res = await scanStore.acknowledgePaths(
					ids.slice(i, i + 500),
					why,
				);
				acknowledged += res?.acknowledged ?? 0;
			}
			toastStore.addToast(
				`${acknowledged} path(s) acknowledged`,
				"success",
			);
		}
		emit("done");
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to save",
			"error",
		);
	} finally {
		saving.value = false;
	}
}
</script>

<template>
	<Teleport to="body">
		<div v-if="mode" class="modal-overlay" @click.self="emit('close')">
			<div class="modal-content card triage-modal">
				<header class="modal-header">
					<h2 v-if="mode === 'allowlist' && isPlugins">
						Allowlist {{ pluginList.length }} plugin{{
							pluginList.length === 1 ? "" : "s"
						}}
					</h2>
					<h2 v-else-if="mode === 'allowlist'">
						Allowlist file content
					</h2>
					<h2 v-else>
						Acknowledge {{ findings.length }} path{{
							findings.length === 1 ? "" : "s"
						}}
					</h2>
					<button class="modal-close" @click="emit('close')">
						<AppIcon name="x" size="20" />
					</button>
				</header>

				<form @submit.prevent="submit">
					<template v-if="mode === 'allowlist' && isPlugins">
						<p class="hint">
							These plugins won't be flagged on any site again,
							including sites that get them later. A plugin with
							the same slug that poses as WordPress still is.
						</p>
						<ul class="path-list">
							<li v-for="p in pluginList" :key="p.slug">
								<code>{{ p.slug }}</code>
								<template v-if="p.name && p.name !== p.slug">
									{{ p.name }}
								</template>
								<span class="sub-text">
									({{ p.sites.join(", ") }})</span
								>
							</li>
						</ul>
					</template>
					<template v-else-if="mode === 'allowlist' && finding">
						<p class="hint">
							Files with this exact content won't be flagged on
							any site again. Anyone can review the entry, and
							removing it later is recorded.
						</p>
						<dl class="triage-detail">
							<dt>From</dt>
							<dd>
								{{ siteDomain(finding.site_id) }}:
								<code>{{ finding.path }}</code>
							</dd>
							<dt>SHA-256</dt>
							<dd>
								<code class="hash">{{ finding.sha256 }}</code>
							</dd>
						</dl>
						<pre v-if="finding.snippet" class="snippet">{{
							finding.snippet
						}}</pre>
					</template>
					<template v-else>
						<p class="hint">
							These rules won't flag the paths below on their
							sites again, whatever the files hold. Use this for
							files that are meant to be there.
						</p>
						<ul class="path-list">
							<li v-for="f in findings" :key="f.id">
								{{ siteDomain(f.site_id) }}:
								<code>{{ f.path }}</code>
								<span class="sub-text">({{ f.rule }})</span>
							</li>
						</ul>
					</template>

					<div class="form-group">
						<label for="triage-reason">Reason</label>
						<input
							id="triage-reason"
							v-model="reason"
							type="text"
							required
							maxlength="1000"
							:placeholder="
								mode === 'acknowledge'
									? 'e.g. Deploy script runs as root'
									: isPlugins
										? 'e.g. Premium plugin from the vendor'
										: 'e.g. WP Migrate DB index.php placeholder'
							"
						/>
					</div>

					<template v-if="mode === 'allowlist'">
						<p v-if="matching === null" class="hint">
							Looking for the same content elsewhere…
						</p>
						<template v-else-if="matching.length">
							<label class="checkbox-label">
								<input
									v-model="closeMatching"
									type="checkbox"
								/>
								Also mark the {{ matching.length }} other open
								finding(s)
								{{
									isPlugins
										? "of these plugins"
										: "with this content"
								}}
								allowlisted
							</label>
							<p class="hint">
								On {{ matchingSites.join(", ") }}
							</p>
						</template>
						<p v-else class="hint">
							No other open finding has
							{{ isPlugins ? "these plugins" : "this content" }}.
						</p>
					</template>

					<template v-if="mode === 'acknowledge'">
						<p v-if="matching === null" class="hint">
							Looking for the same path on other sites…
						</p>
						<template v-else-if="matching.length">
							<label class="checkbox-label">
								<input
									v-model="closeMatching"
									type="checkbox"
								/>
								Also acknowledge the {{ matching.length }} other
								open finding(s) with the same path
							</label>
							<p class="hint">
								On {{ matchingSites.join(", ") }}
							</p>
						</template>
						<p v-else class="hint">
							No other open finding has this path.
						</p>
					</template>

					<div class="form-actions">
						<button
							type="button"
							class="btn btn-outline btn-sm"
							@click="emit('close')"
						>
							Cancel
						</button>
						<button
							type="submit"
							class="btn btn-primary btn-sm"
							:disabled="saving || !reason.trim()"
						>
							{{
								mode === "allowlist"
									? "Allowlist"
									: closeMatching && matching?.length
										? `Acknowledge ${findings.length + matching.length}`
										: "Acknowledge"
							}}
						</button>
					</div>
				</form>
			</div>
		</div>
	</Teleport>
</template>

<style scoped>
.triage-modal {
	max-width: 640px;
}

form {
	display: flex;
	flex-direction: column;
	gap: var(--space-3);

	.form-group,
	.form-actions {
		margin: 0;
	}
}

.hint {
	margin: 0;
	font-size: var(--font-size-xs);
	color: var(--text-muted);
}

.triage-detail {
	display: grid;
	grid-template-columns: max-content 1fr;
	gap: var(--space-1) var(--space-3);
	margin: 0;
	font-size: var(--font-size-xs);

	dt {
		color: var(--text-muted);
	}

	dd {
		margin: 0;
		word-break: break-all;
	}
}

.snippet {
	max-height: 200px;
	overflow: auto;
	margin: 0;
	padding: var(--space-3);
	background: var(--bg-body);
	border: 1px solid var(--border-color);
	border-radius: var(--radius-sm);
	font-size: var(--font-size-xs);
	white-space: pre-wrap;
	word-break: break-all;
}

.path-list {
	max-height: 200px;
	overflow: auto;
	margin: 0;
	padding-left: var(--space-5);
	font-size: var(--font-size-xs);
}
</style>
