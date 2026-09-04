<script setup lang="ts">
import { useConfirm } from "../composables/useConfirm";

const { confirmState, handleConfirm, handleCancel } = useConfirm();
</script>

<template>
	<Teleport to="body">
		<div
			v-if="confirmState.visible"
			class="modal-overlay"
			@click.self="handleCancel"
		>
			<div class="modal-content card">
				<p class="text-main">{{ confirmState.message }}</p>
				<div class="form-actions mt-4">
					<button class="btn btn-outline" @click="handleCancel">
						Cancel
					</button>
					<button
						:class="[
							'btn',
							confirmState.danger ? 'btn-danger' : 'btn-primary',
						]"
						@click="handleConfirm"
					>
						{{ confirmState.confirmLabel }}
					</button>
				</div>
			</div>
		</div>
	</Teleport>
</template>
