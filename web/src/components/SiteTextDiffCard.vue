<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { RouterLink } from "vue-router";
import { useAuthStore } from "../stores/auth";
import { useTextDiffStore } from "../stores/textDiff";
import { useToastStore } from "../stores/toast";
import { useConfirm } from "../composables/useConfirm";
import type { SiteDiffPage, TextDiffMute } from "../types";
import LoadingSpinner from "./LoadingSpinner.vue";
import AppIcon from "./AppIcon.vue";

/**
 * A site's update text diff settings (docs/update-text-diff.md): the extra
 * pages compared besides the frontpage, and the mutes that hide known
 * changes.
 */
const props = defineProps<{
	siteId: string;
	domain: string;
}>();

const authStore = useAuthStore();
const store = useTextDiffStore();
const toast = useToastStore();
const { confirm } = useConfirm();

const pages = ref<SiteDiffPage[]>([]);
const mutes = ref<TextDiffMute[]>([]);
const loading = ref(true);
const error = ref("");
const newPath = ref("");
const adding = ref(false);

async function load() {
	error.value = "";
	try {
		[pages.value, mutes.value] = await Promise.all([
			store.fetchPages(props.siteId),
			store.fetchMutes(props.siteId),
		]);
	} catch (e: any) {
		error.value = e.message || "Failed to load the update check settings";
	} finally {
		loading.value = false;
	}
}

onMounted(load);
watch(() => props.siteId, load);

const now = Date.now();
const activeMutes = computed(() =>
	mutes.value.filter((m) => Date.parse(m.expires_at) > now),
);

async function addPage() {
	let path = newPath.value.trim();
	if (!path) return;
	if (!path.startsWith("/")) path = `/${path}`;
	adding.value = true;
	try {
		const page = await store.addPage(props.siteId, path);
		pages.value = [...pages.value, page].sort((a, b) =>
			a.path.localeCompare(b.path),
		);
		newPath.value = "";
	} catch (e: any) {
		toast.addToast(e.message || "Failed to add the page", "error");
	} finally {
		adding.value = false;
	}
}

async function removePage(page: SiteDiffPage) {
	try {
		await store.deletePage(props.siteId, page.id);
		pages.value = pages.value.filter((p) => p.id !== page.id);
	} catch (e: any) {
		toast.addToast(e.message || "Failed to remove the page", "error");
	}
}

async function removeMute(mute: TextDiffMute) {
	const ok = await confirm(
		`Remove this mute? Changes to “${mute.line}” on ${mute.path} will be reported again.`,
		{ confirmLabel: "Remove", danger: true },
	);
	if (!ok) return;
	try {
		await store.deleteMute(mute.id);
		mutes.value = mutes.value.filter((m) => m.id !== mute.id);
	} catch (e: any) {
		toast.addToast(e.message || "Failed to remove the mute", "error");
	}
}

function formatDate(iso: string) {
	return new Date(iso).toLocaleDateString(undefined, {
		dateStyle: "medium",
	});
}
</script>

<template>
	<section class="card mt-4">
		<div class="card-header">
			<h2>Update Checks</h2>
			<RouterLink :to="{ name: 'update-review' }" class="text-sm"
				>Update review</RouterLink
			>
		</div>
		<p class="text-muted card-intro">
			Plugin, ZIP and core updates compare the visible text of these pages
			before and after the update.
		</p>

		<LoadingSpinner v-if="loading" message="Loading..." />
		<p v-else-if="error" class="form-error">{{ error }}</p>
		<template v-else>
			<h3 class="section-title">Compared pages</h3>
			<ul class="path-list">
				<li>
					<code>/</code>
					<span class="sub-text">Frontpage, always compared</span>
				</li>
				<li v-for="page in pages" :key="page.id">
					<a
						:href="`https://${domain}${page.path}`"
						target="_blank"
						rel="noopener noreferrer"
						><code>{{ page.path }}</code></a
					>
					<span class="sub-text">added by {{ page.created_by }}</span>
					<button
						v-if="authStore.canExecute"
						class="icon-btn icon-btn-sm"
						:aria-label="`Stop comparing ${page.path}`"
						title="Stop comparing"
						@click="removePage(page)"
					>
						<AppIcon name="trash" size="14" />
					</button>
				</li>
			</ul>
			<form
				v-if="authStore.canExecute"
				class="add-path"
				@submit.prevent="addPage"
			>
				<input
					v-model="newPath"
					type="text"
					placeholder="/en/"
					aria-label="Path to compare"
					maxlength="500"
				/>
				<button
					type="submit"
					class="btn btn-outline btn-sm"
					:disabled="adding || !newPath.trim()"
				>
					Add page
				</button>
			</form>

			<h3 class="section-title">Mutes</h3>
			<p v-if="!activeMutes.length" class="text-muted">
				No muted lines. Mute a line from a diff in the update review
				when it changes for a known reason.
			</p>
			<div v-else class="table-container">
				<table class="data-table">
					<thead>
						<tr>
							<th>Page</th>
							<th>Line</th>
							<th>Until</th>
							<th>By</th>
							<th v-if="authStore.canExecute"></th>
						</tr>
					</thead>
					<tbody>
						<tr v-for="m in activeMutes" :key="m.id">
							<td>
								<code>{{ m.path }}</code>
							</td>
							<td>
								<span class="sub-text">{{
									m.direction === "added"
										? "Appearing:"
										: "Disappearing:"
								}}</span>
								{{ m.line }}
								<div v-if="m.reason" class="sub-text">
									{{ m.reason }}
								</div>
							</td>
							<td class="when">{{ formatDate(m.expires_at) }}</td>
							<td>{{ m.created_by }}</td>
							<td v-if="authStore.canExecute">
								<button
									class="icon-btn icon-btn-sm"
									aria-label="Remove mute"
									title="Remove mute"
									@click="removeMute(m)"
								>
									<AppIcon name="trash" size="14" />
								</button>
							</td>
						</tr>
					</tbody>
				</table>
			</div>
		</template>
	</section>
</template>

<style scoped>
.card-intro {
	margin: 0 0 var(--space-3);
}

.section-title {
	font-size: 14px;
	margin: var(--space-4) 0 var(--space-2);
}

.path-list {
	list-style: none;
	margin: 0;
	padding: 0;
}

.path-list li {
	display: flex;
	align-items: center;
	gap: var(--space-2);
	padding: var(--space-1) 0;
}

.add-path {
	display: flex;
	gap: var(--space-2);
	margin-top: var(--space-2);
	max-width: 420px;
}

.add-path input {
	flex: 1;
	min-width: 0;
}

.text-sm {
	font-size: 13px;
}
</style>
