<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useUpdateJobsStore } from "../stores/updateJobs";
import type { SiteUser } from "../types";
import AppIcon from "./AppIcon.vue";

const props = defineProps<{
	visible: boolean;
	users: (SiteUser & { domain: string })[];
}>();

const emit = defineEmits<{
	(e: "close"): void;
	/**
	 * Deletes were queued for these users (keyed "site_id-id"); closing is
	 * up to the parent once all were.
	 */
	(e: "queued", keys: string[], all: boolean): void;
}>();

const jobsStore = useUpdateJobsStore();

const reassign = ref("");
const submitting = ref(false);
/** Users whose delete couldn't be queued, with why. */
const failures = ref<{ user: string; error: string }[]>([]);

const withPosts = computed(() =>
	props.users.filter((u) => (u.post_count ?? 0) > 0),
);
const unknownPosts = computed(() =>
	props.users.filter((u) => u.post_count === undefined),
);

async function submit() {
	submitting.value = true;
	failures.value = [];
	const target = reassign.value.trim();
	const results = await Promise.allSettled(
		props.users.map((u) =>
			jobsStore.enqueueUserDelete(u.site_id, u.id, target),
		),
	);
	const queued: string[] = [];
	results.forEach((r, i) => {
		const u = props.users[i]!;
		if (r.status === "fulfilled") {
			queued.push(`${u.site_id}-${u.id}`);
		} else {
			failures.value.push({
				user: `${u.login} @ ${u.domain}`,
				error: r.reason?.message || "Failed to queue",
			});
		}
	});
	submitting.value = false;
	// Progress and the outcome show in the task center.
	emit("queued", queued, failures.value.length === 0);
}

watch(
	() => props.visible,
	(visible) => {
		if (!visible) return;
		reassign.value = "";
		failures.value = [];
	},
);
</script>

<template>
	<Teleport to="body">
		<div v-if="visible" class="modal-overlay" @click.self="emit('close')">
			<form class="modal-content modal-sm card" @submit.prevent="submit">
				<header class="modal-header">
					<h2>
						Delete {{ users.length }}
						{{ users.length === 1 ? "user" : "users" }}
					</h2>
					<button
						type="button"
						class="modal-close"
						aria-label="Close"
						@click="emit('close')"
					>
						<AppIcon name="x" size="20" />
					</button>
				</header>

				<ul class="sub-text">
					<li v-for="u in users" :key="`${u.site_id}-${u.id}`">
						<strong>{{ u.login }}</strong> &lt;{{ u.email }}&gt; @
						{{ u.domain }}
						<span class="text-muted"
							>({{ u.post_count ?? "?" }} posts)</span
						>
					</li>
				</ul>

				<div class="form-group">
					<label for="delete-reassign">Reassign posts to</label>
					<input
						id="delete-reassign"
						v-model="reassign"
						type="text"
						placeholder="Login or email (optional)"
						autocomplete="off"
						spellcheck="false"
					/>
					<small class="text-muted">
						On each site, the one user with exactly this login or
						email gets the deleted users' posts.
					</small>
				</div>

				<p
					v-if="!reassign.trim() && withPosts.length > 0"
					class="confirm-option-warning"
				>
					{{ withPosts.map((u) => u.login).join(", ") }}
					{{ withPosts.length === 1 ? "has" : "have" }} posts and
					won't be deleted without a user to reassign them to.
				</p>
				<p
					v-else-if="!reassign.trim() && unknownPosts.length > 0"
					class="confirm-option-warning"
				>
					Post counts aren't known for some users; the deletes check
					on the site and refuse users with posts.
				</p>

				<div v-if="failures.length > 0" class="form-error mt-2">
					<p v-for="f in failures" :key="f.user">
						{{ f.user }}: {{ f.error }}
					</p>
				</div>

				<div class="form-actions">
					<button
						type="button"
						class="btn btn-outline"
						@click="emit('close')"
					>
						{{ failures.length > 0 ? "Close" : "Cancel" }}
					</button>
					<button
						v-if="failures.length === 0"
						type="submit"
						class="btn btn-danger"
						:disabled="submitting || users.length === 0"
					>
						{{ submitting ? "Queueing…" : "Delete" }}
					</button>
				</div>
			</form>
		</div>
	</Teleport>
</template>
