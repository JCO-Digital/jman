<script setup lang="ts">
import type { RouteLocationRaw } from "vue-router";
import { useBackLink } from "../composables/useBackLink";

// A page's header: an optional way back, the title, actions on the right,
// and below them the page's tabs (the #tabs slot).
const props = defineProps<{
	title: string;
	/**
	 * Where the back link leads, e.g. { label: "Sites", to: { name:
	 * "sites" } }. Coming from there, it goes back in history instead, so
	 * the list returns as it was left. `from` names other routes that count.
	 */
	back?: {
		label: string;
		to: RouteLocationRaw;
		from?: readonly string[];
	};
}>();

const backLink = props.back
	? useBackLink(() => props.back!.to, props.back.from)
	: null;
</script>

<template>
	<header class="header" :class="{ 'has-tabs': $slots.tabs }">
		<div class="title-area">
			<a
				v-if="back && backLink"
				:href="$router.resolve(backLink.to.value).href"
				class="back-btn"
				@click.prevent="backLink.go()"
			>
				&larr; {{ back.label }}
			</a>
			<h1>{{ title }}</h1>
			<div v-if="$slots.subtitle" class="header-subtitle">
				<slot name="subtitle"></slot>
			</div>
		</div>
		<div class="header-actions">
			<slot name="actions"></slot>
		</div>
	</header>
	<slot name="tabs"></slot>
</template>
