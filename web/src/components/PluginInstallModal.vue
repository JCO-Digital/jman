<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useUpdateJobsStore } from "../stores/updateJobs";
import { useToastStore } from "../stores/toast";
import AppIcon from "./AppIcon.vue";

const props = defineProps<{
	visible: boolean;
	siteId: string;
	siteName: string;
}>();

const emit = defineEmits<{
	(e: "close"): void;
}>();

const jobsStore = useUpdateJobsStore();
const toast = useToastStore();

/** Matches jman-api's limit for uploaded ZIPs. */
const MAX_UPLOAD_MB = 64;

type Mode = "slug" | "url" | "upload";
const modes: { id: Mode; label: string }[] = [
	{ id: "slug", label: "WordPress.org" },
	{ id: "url", label: "ZIP URL" },
	{ id: "upload", label: "Upload ZIP" },
];

const mode = ref<Mode>("slug");
const slug = ref("");
const url = ref("");
const file = ref<File | null>(null);
const activate = ref(true);
const submitting = ref(false);
const error = ref("");

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

const canSubmit = computed(() => {
	if (submitting.value) return false;
	switch (mode.value) {
		case "slug":
			return /^[a-z0-9][a-z0-9_-]*$/.test(slug.value.trim());
		case "url":
			return /^https?:\/\/.+\.zip(\?.*)?$/i.test(url.value.trim());
		case "upload":
			return !!file.value && !fileError.value;
	}
	return false;
});

function onFileChange(ev: Event) {
	const input = ev.target as HTMLInputElement;
	file.value = input.files?.[0] ?? null;
}

async function submit() {
	if (!canSubmit.value) return;
	submitting.value = true;
	error.value = "";
	try {
		if (mode.value === "upload") {
			await jobsStore.enqueueInstall(props.siteId, {
				file: file.value!,
				activate: activate.value,
			});
		} else {
			const source = mode.value === "slug" ? slug.value : url.value;
			await jobsStore.enqueueInstall(props.siteId, {
				source: source.trim(),
				activate: activate.value,
			});
		}
		// Progress and the outcome show in the task center.
		emit("close");
	} catch (e: any) {
		error.value = e.message || "Failed to queue the install";
		toast.addToast(
			`Failed to queue plugin install: ${error.value}`,
			"error",
		);
	} finally {
		submitting.value = false;
	}
}

watch(
	() => props.visible,
	(visible) => {
		if (!visible) return;
		mode.value = "slug";
		slug.value = "";
		url.value = "";
		file.value = null;
		activate.value = true;
		error.value = "";
	},
);
</script>

<template>
	<Teleport to="body">
		<div v-if="visible" class="modal-overlay" @click.self="emit('close')">
			<form class="modal-content modal-sm card" @submit.prevent="submit">
				<header class="modal-header">
					<h2>Install plugin</h2>
					<button
						type="button"
						class="modal-close"
						aria-label="Close"
						@click="emit('close')"
					>
						<AppIcon name="x" size="20" />
					</button>
				</header>

				<p class="text-muted install-intro">
					Installs on <strong>{{ siteName }}</strong> in the
					background.
				</p>

				<nav class="tabs install-tabs">
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

				<div v-if="mode === 'slug'" class="form-group">
					<label for="install-slug">Plugin slug</label>
					<input
						id="install-slug"
						v-model="slug"
						type="text"
						placeholder="e.g. akismet"
						autocomplete="off"
						spellcheck="false"
					/>
					<small class="text-muted">
						The slug from the plugin's wordpress.org URL.
					</small>
				</div>

				<div v-else-if="mode === 'url'" class="form-group">
					<label for="install-url">ZIP URL</label>
					<input
						id="install-url"
						v-model="url"
						type="url"
						placeholder="https://example.com/my-plugin.zip"
						autocomplete="off"
						spellcheck="false"
					/>
					<small class="text-muted">
						The site downloads the ZIP itself, so the URL must be
						reachable from its server.
					</small>
				</div>

				<div v-else class="form-group">
					<label for="install-file">Plugin ZIP</label>
					<input
						id="install-file"
						type="file"
						accept=".zip,application/zip"
						@change="onFileChange"
					/>
					<small v-if="fileError" class="install-error">{{
						fileError
					}}</small>
					<small v-else class="text-muted">
						Up to {{ MAX_UPLOAD_MB }} MB. An existing plugin with
						the same folder name is replaced.
					</small>
				</div>

				<label class="checkbox-label">
					<input v-model="activate" type="checkbox" />
					Activate after installing
				</label>

				<p v-if="error" class="install-error mt-2">{{ error }}</p>

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
						:disabled="!canSubmit"
					>
						{{ submitting ? "Queueing…" : "Install" }}
					</button>
				</div>
			</form>
		</div>
	</Teleport>
</template>

<style scoped>
.install-intro {
	margin: -8px 0 16px;
	font-size: 14px;
}

.install-tabs {
	margin-bottom: 16px;

	& .tab {
		padding: 8px 14px;
		font-size: 14px;
	}
}

.install-error {
	color: var(--error-text);
	font-size: 13px;
}
</style>
