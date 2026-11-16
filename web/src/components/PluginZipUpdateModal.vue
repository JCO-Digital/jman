<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useUpdateJobsStore } from "../stores/updateJobs";
import { useToastStore } from "../stores/toast";
import { compareVersions } from "../utils/vulnList";
import type { StagedPluginZip } from "../types";
import AppIcon from "./AppIcon.vue";

/** A site with the plugin installed, as the plugin page lists it. */
export interface ZipUpdateSite {
	site_id: string;
	site_domain: string;
	version: string;
	status: string;
	isVulnerable: boolean;
	suppressed: boolean;
	locked: boolean;
}

const props = defineProps<{
	visible: boolean;
	slug: string;
	pluginName: string;
	/** Sites the user may update the plugin on. */
	sites: ZipUpdateSite[];
}>();

const emit = defineEmits<{
	(e: "close"): void;
}>();

const jobsStore = useUpdateJobsStore();
const toast = useToastStore();

/** Matches jman-api's limit for plugin ZIPs. */
const MAX_UPLOAD_MB = 64;

type Mode = "upload" | "url" | "site";
const modes: { id: Mode; label: string }[] = [
	{ id: "upload", label: "Upload ZIP" },
	{ id: "url", label: "ZIP URL" },
	{ id: "site", label: "Copy from a site" },
];

type Filter = "vulnerable" | "older" | "all" | "none";

const mode = ref<Mode>("upload");
const url = ref("");
/** The site to copy the plugin's files from. */
const sourceSiteId = ref("");
const file = ref<File | null>(null);
const checking = ref(false);
const submitting = ref(false);
const error = ref("");
const staged = ref<StagedPluginZip | null>(null);
/** Where ZIPs may be downloaded from; null while loading. */
const urlPrefixes = ref<string[] | null>(null);

const filter = ref<Filter>("vulnerable");
/** Sites on a version older than this are picked by the "older" filter. */
const olderThan = ref("");
const selected = ref(new Set<string>());

const fileError = computed(() => {
	if (!file.value) return "";
	if (!file.value.name.toLowerCase().endsWith(".zip")) {
		return "The file must be a .zip file.";
	}
	if (file.value.size > MAX_UPLOAD_MB * 1024 * 1024) {
		return `The file is larger than ${MAX_UPLOAD_MB} MB.`;
	}
	return "";
});

const canCheck = computed(() => {
	if (checking.value) return false;
	if (mode.value === "upload") return !!file.value && !fileError.value;
	if (mode.value === "site") return !!sourceSiteId.value;
	return /^https?:\/\/\S+$/i.test(url.value.trim());
});

/** Sites to copy from, newest version first. */
const sourceSites = computed(() =>
	[...props.sites].sort(
		(a, b) =>
			compareVersions(b.version || "0", a.version || "0") ||
			a.site_domain.localeCompare(b.site_domain),
	),
);

const zipVersion = computed(() => staged.value?.plugin.version ?? "");
const wrongPlugin = computed(
	() => !!staged.value && staged.value.plugin.slug !== props.slug,
);

/** The site the staged ZIP was copied from, which isn't a target. */
const copiedFrom = computed(() => staged.value?.source_site?.id ?? "");

/** Sites with no change already queued or running on the plugin. */
const available = computed(() =>
	props.sites.filter(
		(s) =>
			s.site_id !== copiedFrom.value &&
			!jobsStore.pendingAction(s.site_id, props.slug),
	),
);
const busyCount = computed(
	() =>
		props.sites.filter((s) => s.site_id !== copiedFrom.value).length -
		available.value.length,
);

function isOlder(s: ZipUpdateSite, than: string) {
	return !!than && !!s.version && compareVersions(s.version, than) < 0;
}

function matches(s: ZipUpdateSite, f: Filter) {
	switch (f) {
		case "vulnerable":
			return s.isVulnerable && !s.suppressed;
		case "older":
			return isOlder(s, olderThan.value.trim());
		case "all":
			return true;
	}
	return false;
}

const filterOptions = computed(() => {
	const count = (f: Filter) =>
		available.value.filter((s) => matches(s, f)).length;
	return [
		{
			id: "vulnerable" as Filter,
			label: `Vulnerable versions (${count("vulnerable")})`,
		},
		{
			id: "older" as Filter,
			label: `Older than a version (${count("older")})`,
		},
		{ id: "all" as Filter, label: `All sites (${available.value.length})` },
		{ id: "none" as Filter, label: "Pick sites by hand" },
	];
});

function applyFilter() {
	selected.value = new Set(
		available.value
			.filter((s) => matches(s, filter.value))
			.map((s) => s.site_id),
	);
}

watch([filter, olderThan], applyFilter);

function toggle(siteId: string) {
	const next = new Set(selected.value);
	if (next.has(siteId)) next.delete(siteId);
	else next.add(siteId);
	selected.value = next;
}

const allSelected = computed(
	() =>
		available.value.length > 0 &&
		available.value.every((s) => selected.value.has(s.site_id)),
);
function toggleAll() {
	selected.value = allSelected.value
		? new Set()
		: new Set(available.value.map((s) => s.site_id));
}

const selectedSites = computed(() =>
	available.value.filter((s) => selected.value.has(s.site_id)),
);
/** Selected sites the ZIP would not move to a newer version. */
const notOlderCount = computed(
	() =>
		selectedSites.value.filter(
			(s) => zipVersion.value && !isOlder(s, zipVersion.value),
		).length,
);
const lockedCount = computed(
	() => selectedSites.value.filter((s) => s.locked).length,
);

function onFileChange(ev: Event) {
	const input = ev.target as HTMLInputElement;
	file.value = input.files?.[0] ?? null;
}

async function checkZip() {
	if (!canCheck.value) return;
	checking.value = true;
	error.value = "";
	try {
		staged.value = await jobsStore.stagePluginZip(
			mode.value === "upload"
				? { file: file.value! }
				: mode.value === "site"
					? { site_id: sourceSiteId.value, plugin: props.slug }
					: { url: url.value.trim() },
		);
		olderThan.value = staged.value.plugin.version;
		filter.value = available.value.some((s) => matches(s, "vulnerable"))
			? "vulnerable"
			: "older";
		applyFilter();
	} catch (e: any) {
		error.value = e.message || "Failed to check the ZIP";
	} finally {
		checking.value = false;
	}
}

function changeZip() {
	staged.value = null;
	error.value = "";
}

async function submit() {
	if (!staged.value || wrongPlugin.value || selectedSites.value.length === 0)
		return;
	submitting.value = true;
	error.value = "";
	try {
		const jobs = await jobsStore.enqueueZipUpdate(
			props.slug,
			staged.value.id,
			selectedSites.value.map((s) => s.site_id),
		);
		const from = staged.value.source_site?.name ?? staged.value.filename;
		toast.addToast(
			`Queued updating ${props.pluginName} from ${from} on ${jobs.length} site${jobs.length === 1 ? "" : "s"}.`,
			"info",
		);
		// Progress and the outcome show in the task center.
		emit("close");
	} catch (e: any) {
		error.value = e.message || "Failed to queue the updates";
	} finally {
		submitting.value = false;
	}
}

watch(
	() => props.visible,
	(visible) => {
		if (!visible) return;
		mode.value = "upload";
		url.value = "";
		sourceSiteId.value = sourceSites.value[0]?.site_id ?? "";
		file.value = null;
		staged.value = null;
		error.value = "";
		filter.value = "vulnerable";
		olderThan.value = "";
		selected.value = new Set();
		if (urlPrefixes.value === null) loadUrlPrefixes();
	},
);

async function loadUrlPrefixes() {
	try {
		urlPrefixes.value = await jobsStore.fetchZipUrlPrefixes();
	} catch {
		// The URL tab says the allowed URLs couldn't be loaded; jman-api
		// checks the URL anyway.
		urlPrefixes.value = null;
	}
}

function formatTime(iso: string) {
	return new Date(iso).toLocaleString(undefined, {
		dateStyle: "short",
		timeStyle: "short",
	});
}

function formatSize(bytes: number) {
	return bytes >= 1024 * 1024
		? `${(bytes / 1024 / 1024).toFixed(1)} MB`
		: `${Math.max(1, Math.round(bytes / 1024))} KB`;
}
</script>

<template>
	<Teleport to="body">
		<div v-if="visible" class="modal-overlay" @click.self="emit('close')">
			<form
				class="modal-content modal-lg card"
				@submit.prevent="staged ? submit() : checkZip()"
			>
				<header class="modal-header">
					<h2>Update {{ pluginName }} from a ZIP</h2>
					<button
						type="button"
						class="modal-close"
						aria-label="Close"
						@click="emit('close')"
					>
						<AppIcon name="x" size="20" />
					</button>
				</header>

				<template v-if="!staged">
					<p class="text-muted zip-intro">
						For plugins that don't update through WordPress, such as
						some paid plugins. jman checks the ZIP, then installs it
						over the plugin on the sites you pick. Active plugins
						stay active.
					</p>

					<nav class="tabs zip-tabs">
						<button
							v-for="m in modes"
							:key="m.id"
							type="button"
							class="tab"
							:class="{ active: mode === m.id }"
							@click="mode = m.id"
						>
							{{ m.label }}
						</button>
					</nav>

					<div v-if="mode === 'upload'" class="form-group">
						<label for="zip-file">Plugin ZIP</label>
						<input
							id="zip-file"
							type="file"
							accept=".zip,application/zip"
							@change="onFileChange"
						/>
						<small v-if="fileError" class="form-error">{{
							fileError
						}}</small>
						<small v-else class="text-muted">
							Up to {{ MAX_UPLOAD_MB }} MB.
						</small>
					</div>

					<div v-else-if="mode === 'site'" class="form-group">
						<label for="zip-source">Site</label>
						<select id="zip-source" v-model="sourceSiteId">
							<option
								v-for="s in sourceSites"
								:key="s.site_id"
								:value="s.site_id"
							>
								{{ s.site_domain }} — {{ s.version }}
							</option>
						</select>
						<small class="text-muted">
							jman copies the plugin's installed files from this
							site into a ZIP. To keep malware from spreading, the
							site's latest malware scan must be recent, have no
							open findings and have run after the plugin's files
							last changed. Symlinks, <code>*.log</code> files and
							version control folders aren't copied.
						</small>
					</div>

					<div v-else class="form-group">
						<label for="zip-url">ZIP URL</label>
						<input
							id="zip-url"
							v-model="url"
							type="url"
							:placeholder="
								urlPrefixes?.length
									? `${urlPrefixes[0]}…`
									: 'https://example.com/download/my-plugin.zip'
							"
							:disabled="urlPrefixes?.length === 0"
							autocomplete="off"
							spellcheck="false"
						/>
						<small
							v-if="urlPrefixes?.length === 0"
							class="form-error"
						>
							Downloading from URLs is off: no URLs are allowed
							(<code>pluginZipUrlPrefixes</code> in jman-api's
							config). Upload the ZIP instead.
						</small>
						<small v-else class="text-muted">
							jman-api downloads the ZIP once and sends it to each
							site, so a short-lived download link works.
							<template v-if="urlPrefixes">
								Allowed:
								<template
									v-for="(p, i) in urlPrefixes"
									:key="p"
								>
									<code>{{ p }}</code
									>{{
										i < urlPrefixes.length - 1 ? ", " : ""
									}}
								</template>
							</template>
						</small>
					</div>

					<p v-if="error" class="form-error mt-2">{{ error }}</p>

					<div class="form-actions">
						<button
							type="button"
							class="btn btn-outline"
							@click="emit('close')"
						>
							Cancel
						</button>
						<button
							type="submit"
							class="btn btn-primary"
							:disabled="!canCheck"
						>
							{{
								checking
									? mode === "site"
										? "Copying…"
										: "Checking…"
									: mode === "site"
										? "Copy plugin"
										: "Check ZIP"
							}}
						</button>
					</div>
				</template>

				<template v-else>
					<div class="zip-summary">
						<AppIcon name="box" size="18" />
						<div>
							<div class="font-medium">
								{{ staged.plugin.name }}
								{{ staged.plugin.version || "(no version)" }}
							</div>
							<div v-if="staged.source_site" class="sub-text">
								Copied from {{ staged.source_site.name }}
								(malware scan clean at
								{{ formatTime(staged.source_site.scanned_at) }})
								· {{ formatSize(staged.size) }} zipped
							</div>
							<div v-else class="sub-text">
								{{ staged.filename }} ·
								{{ formatSize(staged.size) }} · installs as
								<code>{{ staged.plugin.slug }}</code>
							</div>
						</div>
						<button
							type="button"
							class="btn btn-text btn-sm"
							@click="changeZip"
						>
							Change
						</button>
					</div>

					<p
						v-if="staged.source_site?.skipped?.length"
						class="confirm-option-warning"
					>
						Not copied (symlinks or special files):
						<code>{{ staged.source_site.skipped.join(", ") }}</code
						>. If the plugin needs them, upload its ZIP instead.
					</p>

					<p v-if="wrongPlugin" class="form-error">
						This ZIP holds the plugin
						<code>{{ staged.plugin.slug }}</code
						>, not <code>{{ slug }}</code
						>. Pick the ZIP of {{ pluginName }}.
					</p>

					<template v-else>
						<div class="form-row">
							<div class="form-group">
								<label for="zip-filter">Sites</label>
								<select id="zip-filter" v-model="filter">
									<option
										v-for="o in filterOptions"
										:key="o.id"
										:value="o.id"
									>
										{{ o.label }}
									</option>
								</select>
							</div>
							<div v-if="filter === 'older'" class="form-group">
								<label for="zip-older">Older than</label>
								<input
									id="zip-older"
									v-model="olderThan"
									type="text"
									autocomplete="off"
									spellcheck="false"
								/>
							</div>
						</div>

						<div class="table-container zip-sites">
							<table class="data-table">
								<thead>
									<tr>
										<th class="zip-check">
											<input
												type="checkbox"
												:checked="allSelected"
												aria-label="Select all sites"
												@change="toggleAll"
											/>
										</th>
										<th>Site</th>
										<th>Version</th>
										<th>Vuln</th>
									</tr>
								</thead>
								<tbody>
									<tr
										v-for="s in available"
										:key="s.site_id"
										class="clickable-row"
										@click="toggle(s.site_id)"
									>
										<td class="zip-check">
											<input
												type="checkbox"
												:checked="
													selected.has(s.site_id)
												"
												:aria-label="`Update on ${s.site_domain}`"
												@click.stop
												@change="toggle(s.site_id)"
											/>
										</td>
										<td class="font-medium">
											{{ s.site_domain }}
											<span
												v-if="s.locked"
												class="status-badge badge-sm warning"
												title="Update-locked: the ZIP is installed regardless"
											>
												<AppIcon
													name="lock"
													size="11"
												/>
												Locked
											</span>
										</td>
										<td>
											<code>{{ s.version }}</code>
											<span
												v-if="
													zipVersion &&
													!isOlder(s, zipVersion)
												"
												class="sub-text"
											>
												{{
													s.version === zipVersion
														? " same"
														: " newer"
												}}
											</span>
										</td>
										<td>
											<span
												v-if="s.isVulnerable"
												class="status-badge badge-sm"
												:class="
													s.suppressed
														? 'warning'
														: 'error'
												"
											>
												{{
													s.suppressed
														? "Suppressed"
														: "Yes"
												}}
											</span>
											<span v-else class="text-muted"
												>—</span
											>
										</td>
									</tr>
								</tbody>
							</table>
						</div>
						<p v-if="busyCount" class="sub-text mt-2">
							{{ busyCount }} site{{
								busyCount === 1 ? "" : "s"
							}}
							with a change already queued on {{ slug }}
							{{ busyCount === 1 ? "is" : "are" }} not listed.
						</p>

						<p v-if="notOlderCount" class="confirm-option-warning">
							{{ notOlderCount }} selected site{{
								notOlderCount === 1 ? " has" : "s have"
							}}
							{{ zipVersion }} or newer; the ZIP replaces it with
							{{ zipVersion }}.
						</p>
						<p v-if="lockedCount" class="confirm-option-warning">
							{{ pluginName }} is update-locked on
							{{ lockedCount }} selected site{{
								lockedCount === 1 ? "" : "s"
							}}. The ZIP is installed there regardless of the
							lock.
						</p>
					</template>

					<p v-if="error" class="form-error mt-2">{{ error }}</p>

					<div class="form-actions">
						<button
							type="button"
							class="btn btn-outline"
							@click="emit('close')"
						>
							Cancel
						</button>
						<button
							type="submit"
							class="btn btn-primary"
							:disabled="
								wrongPlugin ||
								submitting ||
								selectedSites.length === 0
							"
						>
							{{
								submitting
									? "Queueing…"
									: `Update ${selectedSites.length} site${selectedSites.length === 1 ? "" : "s"}`
							}}
						</button>
					</div>
				</template>
			</form>
		</div>
	</Teleport>
</template>

<style scoped>
.zip-intro {
	margin: -8px 0 16px;
	font-size: 14px;
}

.zip-tabs {
	margin-bottom: 16px;

	& .tab {
		padding: 8px 14px;
		font-size: 14px;
	}
}

.zip-summary {
	display: flex;
	align-items: center;
	gap: var(--space-3);
	padding: var(--space-3);
	margin-bottom: 16px;
	border: 1px solid var(--border-color);
	border-radius: 6px;

	& > div {
		flex: 1;
		min-width: 0;
	}
}

.zip-sites {
	max-height: 45vh;
	overflow-y: auto;
}

.zip-check {
	width: 32px;
}
</style>
