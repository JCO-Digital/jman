<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useUpdateJobsStore } from "../stores/updateJobs";
import { useToastStore } from "../stores/toast";
import { generatePassword } from "../utils/generatePassword";
import AppIcon from "./AppIcon.vue";

const props = defineProps<{
	visible: boolean;
	siteId: string;
	siteName: string;
	/** Roles seen on the site, offered besides the standard ones. */
	siteRoles: string[];
}>();

const emit = defineEmits<{
	(e: "close"): void;
}>();

const jobsStore = useUpdateJobsStore();
const toast = useToastStore();

/** Matches jman-api's minimum. */
const MIN_PASSWORD_LENGTH = 12;
const STANDARD_ROLES = ["administrator", "editor", "author", "contributor"];

const login = ref("");
const email = ref("");
const role = ref("administrator");
const password = ref("");
const submitting = ref(false);
const error = ref("");
/** Set once the job is queued: the password to hand over. */
const queued = ref(false);

const roles = computed(() => [
	...STANDARD_ROLES,
	...props.siteRoles.filter((r) => !STANDARD_ROLES.includes(r)).sort(),
]);

const canSubmit = computed(
	() =>
		!submitting.value &&
		/^[A-Za-z0-9_.@][A-Za-z0-9 _.@-]{0,59}$/.test(login.value) &&
		login.value.trim() === login.value &&
		/^[^\s@-][^\s@]*@[^\s@]+\.[^\s@]+$/.test(email.value) &&
		password.value.length >= MIN_PASSWORD_LENGTH,
);

async function copyPassword() {
	try {
		await navigator.clipboard.writeText(password.value);
		toast.addToast("Password copied", "success");
	} catch {
		toast.addToast("Couldn't copy the password", "error");
	}
}

async function submit() {
	if (!canSubmit.value) return;
	submitting.value = true;
	error.value = "";
	try {
		await jobsStore.enqueueUserCreate(props.siteId, {
			login: login.value,
			email: email.value,
			role: role.value,
			password: password.value,
		});
		// Progress and the outcome show in the task center; the password
		// stays here until the modal is closed.
		queued.value = true;
	} catch (e: any) {
		error.value = e.message || "Failed to queue the user";
	} finally {
		submitting.value = false;
	}
}

watch(
	() => props.visible,
	(visible) => {
		if (!visible) return;
		login.value = "";
		email.value = "";
		role.value = "administrator";
		password.value = generatePassword();
		error.value = "";
		queued.value = false;
	},
);
</script>

<template>
	<Teleport to="body">
		<div v-if="visible" class="modal-overlay" @click.self="emit('close')">
			<div v-if="queued" class="modal-content modal-sm card">
				<header class="modal-header">
					<h2>User queued</h2>
					<button
						type="button"
						class="modal-close"
						aria-label="Close"
						@click="emit('close')"
					>
						<AppIcon name="x" size="20" />
					</button>
				</header>
				<p>
					<strong>{{ login }}</strong> is being created on
					<strong>{{ siteName }}</strong
					>; the task center shows when it's done.
				</p>
				<p class="confirm-option-warning">
					Copy the password now. jman doesn't store it, so it isn't
					shown again.
				</p>
				<div class="flex-row gap-2 mt-4">
					<code class="truncate">{{ password }}</code>
					<button
						type="button"
						class="btn btn-outline btn-sm"
						@click="copyPassword"
					>
						<AppIcon name="copy" size="14" />
						Copy
					</button>
				</div>
				<div class="form-actions">
					<button
						type="button"
						class="btn btn-primary"
						@click="emit('close')"
					>
						Done
					</button>
				</div>
			</div>

			<form
				v-else
				class="modal-content modal-sm card"
				@submit.prevent="submit"
			>
				<header class="modal-header">
					<h2>Add user</h2>
					<button
						type="button"
						class="modal-close"
						aria-label="Close"
						@click="emit('close')"
					>
						<AppIcon name="x" size="20" />
					</button>
				</header>

				<p class="text-muted">
					Creates a WordPress user on <strong>{{ siteName }}</strong>
					in the background.
				</p>

				<div class="form-group">
					<label for="user-login">Username</label>
					<input
						id="user-login"
						v-model="login"
						type="text"
						autocomplete="off"
						spellcheck="false"
					/>
					<small class="text-muted">
						Letters, digits, spaces and _ . - @.
					</small>
				</div>

				<div class="form-group">
					<label for="user-email">Email</label>
					<input
						id="user-email"
						v-model="email"
						type="email"
						autocomplete="off"
						spellcheck="false"
					/>
				</div>

				<div class="form-group">
					<label for="user-role">Role</label>
					<select id="user-role" v-model="role">
						<option v-for="r in roles" :key="r" :value="r">
							{{ r }}
						</option>
					</select>
				</div>

				<div class="form-group">
					<label for="user-password">Password</label>
					<div class="flex-row gap-2">
						<input
							id="user-password"
							v-model="password"
							type="text"
							autocomplete="new-password"
							spellcheck="false"
						/>
						<button
							type="button"
							class="btn btn-outline btn-sm"
							title="Generate a new password"
							@click="password = generatePassword()"
						>
							<AppIcon name="refresh" size="14" />
						</button>
						<button
							type="button"
							class="btn btn-outline btn-sm"
							title="Copy the password"
							@click="copyPassword"
						>
							<AppIcon name="copy" size="14" />
						</button>
					</div>
					<small class="text-muted">
						At least {{ MIN_PASSWORD_LENGTH }} characters. jman
						doesn't store it.
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
						:disabled="!canSubmit"
					>
						{{ submitting ? "Queueing…" : "Add user" }}
					</button>
				</div>
			</form>
		</div>
	</Teleport>
</template>
