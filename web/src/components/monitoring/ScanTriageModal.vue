<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useScanStore } from "../../stores/scan";
import { useDataStore } from "../../stores/data";
import { useToastStore } from "../../stores/toast";
import AppIcon from "../AppIcon.vue";
import type { ScanFinding } from "../../types";

interface Props {
	/** allowlist trusts findings[0]'s hash; acknowledge trusts the paths. */
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

watch(
	() => props.mode,
	async (mode) => {
		reason.value = "";
		closeMatching.value = false;
		matching.value = null;
		if (mode !== "allowlist" || !finding.value?.sha256) return;
		const target = finding.value;
		try {
			const found = await scanStore.fetchFindings({
				sha256: target.sha256,
				status: "open",
				limit: 1000,
			});
			if (finding.value === target) {
				matching.value = found.filter((f) => f.id !== target.id);
			}
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
	const target = finding.value;
	if (!why || !props.mode || !target) return;
	saving.value = true;
	try {
		if (props.mode === "allowlist") {
			const res = await scanStore.addAllowlist(
				target.id,
				why,
				closeMatching.value,
			);
			toastStore.addToast(
				`Hash allowlisted; ${res?.allowlisted ?? 0} finding(s) marked allowlisted`,
				"success",
			);
		} else {
			const res = await scanStore.acknowledgePaths(
				props.findings.map((f) => f.id),
				why,
			);
			toastStore.addToast(
				`${res?.acknowledged ?? 0} path(s) acknowledged`,
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
					<h2 v-if="mode === 'allowlist'">Allowlist file content</h2>
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
					<template v-if="mode === 'allowlist' && finding">
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
								mode === 'allowlist'
									? 'e.g. WP Migrate DB index.php placeholder'
									: 'e.g. Deploy script runs as root'
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
								finding(s) with this content allowlisted
							</label>
							<p class="hint">
								On {{ matchingSites.join(", ") }}
							</p>
						</template>
						<p v-else class="hint">
							No other open finding has this content.
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
