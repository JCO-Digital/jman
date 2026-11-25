<script setup lang="ts">
import { computed } from "vue";
import {
	useDesktopNotifyStore,
	type DesktopNotifySettings,
} from "../../stores/desktopNotify";

const store = useDesktopNotifyStore();

const options: {
	key: keyof DesktopNotifySettings;
	label: string;
	hint: string;
}[] = [
	{
		key: "jobs",
		label: "My jobs finishing",
		hint: "One notification per batch of updates and other jobs, with any failures. Only while jman is in the background.",
	},
	{
		key: "reviews",
		label: "Update review results",
		hint: "When the text check after an update finds changes to review, even with the option above off.",
	},
	{
		key: "incidents",
		label: "Incidents",
		hint: "A site going down or a new security incident, whether or not jman is in front.",
	},
];

const blocked = computed(() => store.permission === "denied");
const anyOn = computed(() => Object.values(store.settings).some(Boolean));

function onChange(key: keyof DesktopNotifySettings, e: Event) {
	const on = (e.target as HTMLInputElement).checked;
	// Stays unticked if the browser says no.
	(e.target as HTMLInputElement).checked = store.settings[key];
	void store.setEnabled(key, on);
}
</script>

<template>
	<section class="card">
		<h2>Desktop Notifications</h2>
		<p class="sub-text mb-4">
			Notifications from this browser while a jman tab is open, also when
			it's in the background. They're set for this browser only.
		</p>

		<p v-if="!store.supported" class="notice">
			This browser doesn't support desktop notifications.
		</p>
		<template v-else>
			<p v-if="blocked" class="notice">
				Notifications are blocked for jman in this browser. Allow them
				in the site settings (the icon left of the address), then
				reload.
			</p>
			<div class="options">
				<label v-for="o in options" :key="o.key" class="option">
					<input
						type="checkbox"
						:checked="store.settings[o.key]"
						:disabled="blocked"
						@change="onChange(o.key, $event)"
					/>
					<span>
						<span class="option-label">{{ o.label }}</span>
						<span class="option-hint">{{ o.hint }}</span>
					</span>
				</label>
			</div>
			<button
				v-if="anyOn && store.permission === 'granted'"
				class="btn btn-outline btn-sm mt-4"
				@click="store.test()"
			>
				Send a test notification
			</button>
		</template>
	</section>
</template>

<style scoped>
.options {
	display: flex;
	flex-direction: column;
	gap: 12px;
	max-width: 640px;
}

.option {
	display: flex;
	align-items: flex-start;
	gap: 10px;
	cursor: pointer;

	& input {
		margin-top: 3px;
	}
	& > span {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
}

.option-label {
	font-weight: 500;
	color: var(--text-heading);
}

.option-hint {
	font-size: 13px;
	color: var(--text-muted);
}

.notice {
	padding: 10px 12px;
	border-radius: 6px;
	background: var(--warning-bg);
	color: var(--warning-text);
	font-size: 14px;
	max-width: 640px;
}
</style>
