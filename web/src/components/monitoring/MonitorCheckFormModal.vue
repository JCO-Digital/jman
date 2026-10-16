<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useMonitorStore } from "../../stores/monitor";
import { useToastStore } from "../../stores/toast";
import AppIcon from "../AppIcon.vue";
import type {
	MonitorCheck,
	MonitorCheckPayload,
	MonitorCheckTestResult,
} from "../../types";

interface Props {
	visible: boolean;
	/** The check to edit; null creates a new one. */
	editCheck?: MonitorCheck | null;
}

const props = withDefaults(defineProps<Props>(), { editCheck: null });

const emit = defineEmits<{
	(e: "close"): void;
	(e: "saved", check: MonitorCheck): void;
}>();

const monitorStore = useMonitorStore();
const toastStore = useToastStore();

type ValueType = "string" | "number" | "boolean" | "null";
type CacheBypass = "global" | "on" | "off";

interface SecretRow {
	name: string;
	value: string;
}

const name = ref("");
const host = ref("");
const path = ref("/");
const expect = ref<"html" | "json">("html");
const assertOp = ref<"" | "equals" | "exists">("");
const assertPath = ref("");
const valueType = ref<ValueType>("boolean");
const valueText = ref("");
const valueBool = ref("true");
const cacheBypass = ref<CacheBypass>("global");
/** New or replaced secret values, sent on save. */
const secretRows = ref<SecretRow[]>([]);
/** Stored secret headers to remove on save. */
const removedSecrets = ref<Set<string>>(new Set());

const isSubmitting = ref(false);
const isTesting = ref(false);
const errorMessage = ref<string | null>(null);
const testResult = ref<MonitorCheckTestResult | null>(null);

const isEdit = computed(() => !!props.editCheck);
const isDefault = computed(() => !!props.editCheck?.is_default);
const title = computed(() =>
	isEdit.value
		? `Edit check ${props.editCheck?.name || props.editCheck?.domain || ""}`
		: "Add Check",
);
const storedSecrets = computed(() => props.editCheck?.secret_headers ?? []);

watch(
	() => props.visible,
	(open) => {
		if (!open) return;
		const c = props.editCheck;
		errorMessage.value = null;
		testResult.value = null;
		name.value = c?.name ?? "";
		host.value = c?.is_default ? (c?.domain ?? "") : (c?.host ?? "");
		path.value = c?.path ?? "/";
		expect.value = c?.expect ?? "html";
		assertOp.value = c?.assert_op ?? "";
		assertPath.value = c?.assert_path ?? "";
		setAssertValue(c?.assert_value);
		cacheBypass.value =
			c?.cache_bypass === undefined || c?.cache_bypass === null
				? "global"
				: c.cache_bypass
					? "on"
					: "off";
		secretRows.value = [];
		removedSecrets.value = new Set();
	},
);

function setAssertValue(v: unknown) {
	valueText.value = "";
	valueBool.value = "true";
	if (v === undefined) {
		valueType.value = "boolean";
	} else if (v === null) {
		valueType.value = "null";
	} else if (typeof v === "boolean") {
		valueType.value = "boolean";
		valueBool.value = String(v);
	} else if (typeof v === "number") {
		valueType.value = "number";
		valueText.value = String(v);
	} else if (typeof v === "string") {
		valueType.value = "string";
		valueText.value = v;
	} else {
		// Objects and arrays aren't editable here; keep them as text.
		valueType.value = "string";
		valueText.value = JSON.stringify(v);
	}
}

/** The typed expected value, or an error message. */
function assertValue(): { value?: unknown; error?: string } {
	switch (valueType.value) {
		case "null":
			return { value: null };
		case "boolean":
			return { value: valueBool.value === "true" };
		case "number": {
			const n = Number(valueText.value);
			if (valueText.value.trim() === "" || !Number.isFinite(n)) {
				return { error: "The expected value must be a number." };
			}
			return { value: n };
		}
		default:
			return { value: valueText.value };
	}
}

function buildPayload(): MonitorCheckPayload | null {
	const payload: MonitorCheckPayload = {
		name: name.value.trim(),
		path: path.value.trim() || "/",
		expect: expect.value,
		cache_bypass:
			cacheBypass.value === "global" ? null : cacheBypass.value === "on",
	};
	if (!isDefault.value) payload.host = host.value.trim();

	if (expect.value === "json" && assertOp.value) {
		payload.assert_op = assertOp.value;
		payload.assert_path = assertPath.value.trim();
		if (assertOp.value === "equals") {
			const { value, error } = assertValue();
			if (error) {
				errorMessage.value = error;
				return null;
			}
			payload.assert_value = value;
		} else {
			payload.assert_value = null;
		}
	} else {
		payload.assert_op = "";
		payload.assert_path = "";
		payload.assert_value = null;
	}
	return payload;
}

/** Secret values entered in the form, by header name. */
function enteredSecrets(): Record<string, string> {
	const out: Record<string, string> = {};
	for (const row of secretRows.value) {
		if (row.name.trim() && row.value) out[row.name.trim()] = row.value;
	}
	return out;
}

function addSecretRow(name = "") {
	secretRows.value.push({ name, value: "" });
}

function toggleRemoveSecret(name: string) {
	const next = new Set(removedSecrets.value);
	if (next.has(name)) next.delete(name);
	else next.add(name);
	removedSecrets.value = next;
}

async function runTest() {
	errorMessage.value = null;
	const payload = buildPayload();
	if (!payload) return;
	isTesting.value = true;
	testResult.value = null;
	try {
		testResult.value = await monitorStore.testCheck({
			...payload,
			id: props.editCheck?.id,
			secret_headers: enteredSecrets(),
		});
	} catch (e) {
		errorMessage.value = e instanceof Error ? e.message : "Test failed";
	} finally {
		isTesting.value = false;
	}
}

async function handleSubmit() {
	errorMessage.value = null;
	const payload = buildPayload();
	if (!payload) return;
	isSubmitting.value = true;
	try {
		let saved = props.editCheck
			? await monitorStore.updateCheck(props.editCheck.id, payload)
			: await monitorStore.createCheck(payload);
		for (const header of removedSecrets.value) {
			saved = await monitorStore.deleteCheckSecret(saved.id, header);
		}
		for (const [header, value] of Object.entries(enteredSecrets())) {
			saved = await monitorStore.setCheckSecret(saved.id, header, value);
		}
		toastStore.addToast(
			isEdit.value ? "Check saved" : "Check added",
			"success",
		);
		emit("saved", saved);
		emit("close");
	} catch (e) {
		errorMessage.value =
			e instanceof Error ? e.message : "Failed to save the check";
	} finally {
		isSubmitting.value = false;
	}
}

async function handleReset() {
	if (!props.editCheck) return;
	isSubmitting.value = true;
	errorMessage.value = null;
	try {
		const saved = await monitorStore.resetCheck(props.editCheck.id);
		toastStore.addToast("Check reset to its defaults", "success");
		emit("saved", saved);
		emit("close");
	} catch (e) {
		errorMessage.value =
			e instanceof Error ? e.message : "Failed to reset the check";
	} finally {
		isSubmitting.value = false;
	}
}

function formatDate(iso: string): string {
	return new Date(iso).toLocaleString(undefined, {
		month: "short",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
	});
}
</script>

<template>
	<Teleport to="body">
		<div v-if="visible" class="modal-overlay" @click.self="emit('close')">
			<div class="modal-content card check-form-modal">
				<header class="modal-header">
					<h2>{{ title }}</h2>
					<button class="modal-close" @click="emit('close')">
						<AppIcon name="x" size="20" />
					</button>
				</header>

				<div v-if="errorMessage" class="error-banner">
					<p>{{ errorMessage }}</p>
				</div>

				<form @submit.prevent="handleSubmit">
					<div class="form-group">
						<label for="check-name">Name</label>
						<input
							id="check-name"
							v-model="name"
							type="text"
							placeholder="Optional; shown in alerts"
							maxlength="200"
						/>
					</div>

					<div class="form-row">
						<div class="form-group">
							<label for="check-host">Host</label>
							<input
								id="check-host"
								v-model="host"
								type="text"
								placeholder="api.example.com"
								:disabled="isDefault"
								:required="!isDefault"
								autocomplete="off"
							/>
							<span v-if="isDefault" class="hint">
								A site's default check uses the site's domain.
							</span>
						</div>
						<div class="form-group">
							<label for="check-path">Path</label>
							<input
								id="check-path"
								v-model="path"
								type="text"
								placeholder="/healthz"
								autocomplete="off"
							/>
						</div>
					</div>

					<div class="form-row">
						<div class="form-group">
							<label for="check-expect">Expect</label>
							<select id="check-expect" v-model="expect">
								<option value="html">HTML page</option>
								<option value="json">JSON</option>
							</select>
						</div>
						<div class="form-group">
							<label for="check-cache">Cache bypass</label>
							<select id="check-cache" v-model="cacheBypass">
								<option value="global">Global setting</option>
								<option value="on">On</option>
								<option value="off">Off</option>
							</select>
						</div>
					</div>
					<p class="hint">
						A 2xx response is always required.
						{{
							expect === "html"
								? "The body must look like an HTML page."
								: "The body must be valid JSON."
						}}
					</p>

					<fieldset v-if="expect === 'json'" class="assertion">
						<legend>Assertion</legend>
						<div class="form-row">
							<div class="form-group">
								<label for="check-assert-op">Check</label>
								<select id="check-assert-op" v-model="assertOp">
									<option value="">None</option>
									<option value="equals">Value equals</option>
									<option value="exists">Field exists</option>
								</select>
							</div>
							<div v-if="assertOp" class="form-group">
								<label for="check-assert-path">Field</label>
								<input
									id="check-assert-path"
									v-model="assertPath"
									type="text"
									placeholder="status or items.0.state"
									required
								/>
							</div>
						</div>
						<div v-if="assertOp === 'equals'" class="form-row">
							<div class="form-group">
								<label for="check-value-type">Value type</label>
								<select
									id="check-value-type"
									v-model="valueType"
								>
									<option value="boolean">Boolean</option>
									<option value="string">String</option>
									<option value="number">Number</option>
									<option value="null">null</option>
								</select>
							</div>
							<div class="form-group">
								<label for="check-value">Expected value</label>
								<select
									v-if="valueType === 'boolean'"
									id="check-value"
									v-model="valueBool"
								>
									<option value="true">true</option>
									<option value="false">false</option>
								</select>
								<input
									v-else-if="valueType !== 'null'"
									id="check-value"
									v-model="valueText"
									:type="
										valueType === 'number'
											? 'number'
											: 'text'
									"
									step="any"
								/>
								<span v-else class="hint"
									>The field must be null.</span
								>
							</div>
						</div>
						<p v-if="assertOp === 'equals'" class="hint">
							Types matter: the boolean <code>true</code> doesn't
							equal the string <code>"true"</code>.
						</p>
					</fieldset>

					<fieldset class="secrets">
						<legend>Secret headers</legend>
						<p class="hint">
							Sent with every request and never shown again. They
							aren't sent to another host after a redirect.
						</p>
						<ul v-if="storedSecrets.length" class="secret-list">
							<li
								v-for="secret in storedSecrets"
								:key="secret.name"
								:class="{
									removed: removedSecrets.has(secret.name),
								}"
							>
								<code>{{ secret.name }}</code>
								<span class="hint">
									set, updated
									{{ formatDate(secret.updated_at) }}
									<template v-if="secret.updated_by">
										by {{ secret.updated_by }}
									</template>
								</span>
								<span class="secret-actions">
									<button
										type="button"
										class="btn btn-text btn-sm"
										@click="addSecretRow(secret.name)"
									>
										Replace
									</button>
									<button
										type="button"
										class="btn btn-text btn-sm"
										@click="toggleRemoveSecret(secret.name)"
									>
										{{
											removedSecrets.has(secret.name)
												? "Keep"
												: "Remove"
										}}
									</button>
								</span>
							</li>
						</ul>
						<div
							v-for="(row, i) in secretRows"
							:key="i"
							class="form-row secret-row"
						>
							<input
								v-model="row.name"
								type="text"
								placeholder="Header name, e.g. Authorization"
								autocomplete="off"
							/>
							<input
								v-model="row.value"
								type="password"
								placeholder="Value"
								autocomplete="new-password"
							/>
							<button
								type="button"
								class="icon-btn"
								title="Discard"
								@click="secretRows.splice(i, 1)"
							>
								<AppIcon name="x" size="16" />
							</button>
						</div>
						<button
							type="button"
							class="btn btn-text btn-sm"
							@click="addSecretRow()"
						>
							+ Add header
						</button>
					</fieldset>

					<div
						v-if="testResult"
						class="test-result"
						:class="testResult.up ? 'test-up' : 'test-down'"
					>
						<strong>{{ testResult.up ? "Up" : "Down" }}</strong>
						<span v-if="testResult.status_code">
							HTTP {{ testResult.status_code }}
						</span>
						<span>{{ testResult.duration_ms }} ms</span>
						<span v-if="testResult.message">{{
							testResult.message
						}}</span>
						<code class="test-url">{{ testResult.url }}</code>
					</div>

					<div class="form-actions mt-4">
						<button
							v-if="isDefault"
							type="button"
							class="btn btn-text"
							:disabled="isSubmitting"
							title="Path /, expect HTML, no assertion, global cache bypass"
							@click="handleReset"
						>
							Reset to default
						</button>
						<span class="spacer"></span>
						<button
							type="button"
							class="btn btn-outline"
							:disabled="isTesting"
							@click="runTest"
						>
							{{ isTesting ? "Testing..." : "Test" }}
						</button>
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
							:disabled="isSubmitting"
						>
							{{
								isSubmitting
									? "Saving..."
									: isEdit
										? "Save Changes"
										: "Add Check"
							}}
						</button>
					</div>
				</form>
			</div>
		</div>
	</Teleport>
</template>

<style scoped>
.check-form-modal {
	max-width: 680px;
}

form {
	display: flex;
	flex-direction: column;
	gap: 12px;
}

.hint {
	font-size: 12px;
	color: var(--text-muted);
	margin: 0;
}

fieldset {
	border: 1px solid var(--border-color);
	border-radius: 6px;
	padding: 12px;
	display: flex;
	flex-direction: column;
	gap: 10px;
}

legend {
	font-size: 13px;
	font-weight: 600;
	padding: 0 4px;
	color: var(--text-heading);
}

.secret-list {
	list-style: none;
	margin: 0;
	padding: 0;
	display: flex;
	flex-direction: column;
	gap: 6px;
}

.secret-list li {
	display: flex;
	align-items: center;
	gap: 8px;
	flex-wrap: wrap;
}

.secret-list li.removed code {
	text-decoration: line-through;
	opacity: 0.6;
}

.secret-actions {
	margin-left: auto;
	display: inline-flex;
	gap: 4px;
}

.secret-row {
	align-items: center;
}

.test-result {
	display: flex;
	flex-wrap: wrap;
	align-items: baseline;
	gap: 8px 12px;
	padding: 10px 12px;
	border-radius: 6px;
	font-size: 13px;
}

.test-up {
	background: rgba(22, 163, 74, 0.1);
	border: 1px solid rgba(22, 163, 74, 0.3);
}

.test-down {
	background: rgba(220, 38, 38, 0.08);
	border: 1px solid rgba(220, 38, 38, 0.3);
}

.test-url {
	flex-basis: 100%;
	font-size: 12px;
	color: var(--text-muted);
	word-break: break-all;
}

.form-actions {
	display: flex;
	align-items: center;
	gap: 8px;
	flex-wrap: wrap;
}

.spacer {
	flex: 1;
}
</style>
