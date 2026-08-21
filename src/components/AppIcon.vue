<script setup lang="ts">
import { computed, type Component } from "vue";
import IconCheck from "./icons/IconCheck.vue";
import IconChevronRight from "./icons/IconChevronRight.vue";
import IconCopy from "./icons/IconCopy.vue";
import IconCreditCard from "./icons/IconCreditCard.vue";
import IconDragHandle from "./icons/IconDragHandle.vue";
import IconEdit from "./icons/IconEdit.vue";
import IconExternalLink from "./icons/IconExternalLink.vue";
import IconNote from "./icons/IconNote.vue";
import IconOrganization from "./icons/IconOrganization.vue";
import IconPlugin from "./icons/IconPlugin.vue";
import IconPlusCircle from "./icons/IconPlusCircle.vue";
import IconRefresh from "./icons/IconRefresh.vue";
import IconSettings from "./icons/IconSettings.vue";
import IconSite from "./icons/IconSite.vue";
import IconTag from "./icons/IconTag.vue";
import IconTask from "./icons/IconTask.vue";
import IconTrash from "./icons/IconTrash.vue";
import IconVulnerability from "./icons/IconVulnerability.vue";
import IconX from "./icons/IconX.vue";

const props = defineProps<{
	name: string;
	size?: string | number;
}>();

const iconMap: Record<string, Component> = {
	check: IconCheck,
	"chevron-right": IconChevronRight,
	copy: IconCopy,
	"credit-card": IconCreditCard,
	"drag-handle": IconDragHandle,
	edit: IconEdit,
	"external-link": IconExternalLink,
	note: IconNote,
	organization: IconOrganization,
	plugin: IconPlugin,
	"plus-circle": IconPlusCircle,
	refresh: IconRefresh,
	settings: IconSettings,
	site: IconSite,
	tag: IconTag,
	task: IconTask,
	trash: IconTrash,
	vulnerability: IconVulnerability,
	x: IconX,
};

const icon = computed(() => iconMap[props.name]);

const sizeStyle = computed(() => {
	const s = props.size || "1em";
	const value = typeof s === "number" ? `${s}px` : s;
	return {
		display: "inline-flex",
		width: value,
		height: value,
		minWidth: value,
		minHeight: value,
		flexShrink: 0,
	};
});
</script>

<template>
	<span
		class="app-icon"
		:style="sizeStyle"
		role="img"
		:aria-label="name + ' icon'"
	>
		<component :is="icon" v-if="icon" />
	</span>
</template>

<style scoped>
.app-icon {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	vertical-align: middle;
	line-height: 1;
}

.app-icon :deep(svg) {
	width: 100%;
	height: 100%;
	display: block;
}
</style>
