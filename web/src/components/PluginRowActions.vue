<script setup lang="ts">
import { computed, ref } from "vue";
import { useUpdateJobsStore } from "../stores/updateJobs";
import { useToastStore } from "../stores/toast";
import { useConfirm } from "../composables/useConfirm";
import { availablePluginUpdate } from "../utils/format";
import type { PluginActionKind, UpdateJobKind } from "../types";

/**
 * Update / activate / deactivate / delete buttons for one plugin on one
 * site. Each queues a background job; while one is queued or running the
 * buttons give way to its progress.
 */
const props = defineProps<{
	siteId: string;
	siteName: string;
	plugin: { name: string; status: string; version: string; update: string };
}>();

const jobsStore = useUpdateJobsStore();
const toast = useToastStore();
const { confirm, confirmWithOption } = useConfirm();

const PENDING_LABELS: Partial<Record<UpdateJobKind, string>> = {
	plugins: "Updating…",
	activate: "Activating…",
	deactivate: "Deactivating…",
	delete: "Deleting…",
	uninstall: "Uninstalling…",
};

// True while the request that queues a job is in flight.
const queueing = ref(false);
const pending = computed(() =>
	jobsStore.pendingAction(props.siteId, props.plugin.name),
);
const updateTo = computed(() => availablePluginUpdate(props.plugin));
const status = computed(() => props.plugin.status);

async function queue(what: string, fn: () => Promise<unknown>) {
	queueing.value = true;
	try {
		await fn();
	} catch (e: any) {
		toast.addToast(
			`Failed to queue ${what} of ${props.plugin.name}: ${e.message}`,
			"error",
		);
	} finally {
		queueing.value = false;
	}
}

function action(kind: PluginActionKind) {
	return queue(kind, () =>
		jobsStore.enqueueAction(props.siteId, kind, [props.plugin.name]),
	);
}

function update() {
	return queue("update", () =>
		jobsStore.enqueue([
			{ site_id: props.siteId, plugins: [props.plugin.name] },
		]),
	);
}

async function deactivate() {
	const ok = await confirm(
		`Deactivate ${props.plugin.name} on ${props.siteName}?`,
		{ confirmLabel: "Deactivate" },
	);
	if (ok) await action("deactivate");
}

async function remove() {
	const { confirmed, checked } = await confirmWithOption(
		`Delete ${props.plugin.name} from ${props.siteName}? Its files are removed; its settings stay in the database, so reinstalling restores them.`,
		{
			confirmLabel: "Delete",
			danger: true,
			optionLabel:
				"Also remove the plugin's data (runs its uninstall routine)",
			optionWarning:
				"This usually deletes the plugin's settings and database tables, and can't be undone.",
		},
	);
	if (confirmed) await action(checked ? "uninstall" : "delete");
}
</script>

<template>
	<div class="plugin-actions" @click.stop>
		<span v-if="pending" class="plugin-actions-pending">
			<span class="spinner spinner-small"></span>
			{{ PENDING_LABELS[pending] ?? "Working…" }}
		</span>
		<template v-else>
			<button
				v-if="updateTo"
				class="btn btn-primary btn-sm"
				:disabled="queueing"
				:title="`Update ${plugin.version} → ${updateTo}`"
				@click="update"
			>
				Update
			</button>
			<button
				v-if="status === 'active'"
				class="btn btn-outline btn-sm"
				:disabled="queueing"
				@click="deactivate"
			>
				Deactivate
			</button>
			<template v-else-if="status === 'inactive'">
				<button
					class="btn btn-outline btn-sm"
					:disabled="queueing"
					@click="action('activate')"
				>
					Activate
				</button>
				<button
					class="btn btn-outline btn-sm plugin-action-danger"
					:disabled="queueing"
					@click="remove"
				>
					Delete
				</button>
			</template>
		</template>
	</div>
</template>
