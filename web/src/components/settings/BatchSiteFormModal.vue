<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useManagedSitesStore } from "../../stores/managedSites";
import AppIcon from "../AppIcon.vue";
import type {
	ManagedConnectionType,
	ManagedSitePayload,
	ManagedSiteStatus,
	SiteEnvironment,
} from "../../types";

/** Matches maxBatchSites in jman-api. */
const MAX_BATCH_SITES = 500;

interface ParsedLine {
	/** 1-based line number in the textarea. */
	line: number;
	raw: string;
	domain: string;
	sshUser?: string;
	sshHost?: string;
	sshPort?: number;
	sitePath?: string;
	/** Problem found before submitting; blocks the batch until fixed. */
	error?: string;
}

const props = defineProps<{
	visible: boolean;
}>();

const emit = defineEmits<{
	(e: "close"): void;
	(e: "saved", count: number): void;
}>();

const managedSitesStore = useManagedSitesStore();

const input = ref("");
const serverId = ref("");
const provider = ref("manual");
const environment = ref<SiteEnvironment>("production");
const status = ref<ManagedSiteStatus>("active");
const connectionType = ref<ManagedConnectionType>("ssh");
const sshHost = ref("");
const sshPort = ref<number | "">(22);
const sshUser = ref("");
const sitePath = ref("files");
const isWordpress = ref(true);
const canWPCLI = ref(true);
const hasAgent = ref(false);
const isSubmitting = ref(false);
const errorMessage = ref<string | null>(null);
/** Errors jman-api returned for the last submit, keyed by lower-cased domain. */
const submitErrors = ref(new Map<string, string>());

const serverOptions = computed(() => managedSitesStore.servers);

const existingDomains = computed(
	() => new Set(managedSitesStore.sites.map((s) => s.domain.toLowerCase())),
);

const SSH_TARGET_RE = /^(?:([^@\s]+)@)?([^@:\s]+)(?::(\d+))?$/;

function normalizeDomain(value: string): string {
	return value
		.replace(/^https?:\/\//i, "")
		.split("/")[0]!
		.toLowerCase();
}

function parseLine(raw: string, line: number): ParsedLine {
	const fields = raw.split(/[\s,;]+/).filter(Boolean);
	const parsed: ParsedLine = {
		line,
		raw,
		domain: normalizeDomain(fields[0] ?? ""),
	};

	if (!parsed.domain || /[@:]/.test(parsed.domain)) {
		parsed.error = "Not a domain";
		return parsed;
	}
	if (fields.length > 3) {
		parsed.error = "Too many fields";
		return parsed;
	}

	const target = fields[1];
	if (target && target !== "-") {
		const match = SSH_TARGET_RE.exec(target);
		const port = match?.[3] ? Number(match[3]) : undefined;
		if (!match || (port !== undefined && (port < 1 || port > 65535))) {
			parsed.error = "SSH target must be [user@]host[:port]";
			return parsed;
		}
		parsed.sshUser = match[1];
		parsed.sshHost = match[2];
		parsed.sshPort = port;
	}

	const path = fields[2];
	if (path && path !== "-") parsed.sitePath = path;

	return parsed;
}

const parsedLines = computed<ParsedLine[]>(() => {
	const seen = new Set<string>();
	const lines: ParsedLine[] = [];
	input.value.split("\n").forEach((raw, i) => {
		const trimmed = raw.trim();
		if (!trimmed || trimmed.startsWith("#")) return;
		const parsed = parseLine(trimmed, i + 1);
		if (!parsed.error) {
			if (existingDomains.value.has(parsed.domain)) {
				parsed.error = "Already exists";
			} else if (seen.has(parsed.domain)) {
				parsed.error = "Duplicate";
			}
			seen.add(parsed.domain);
		}
		lines.push(parsed);
	});
	return lines;
});

const invalidCount = computed(
	() => parsedLines.value.filter((l) => l.error).length,
);

const canSubmit = computed(
	() =>
		!isSubmitting.value &&
		parsedLines.value.length > 0 &&
		parsedLines.value.length <= MAX_BATCH_SITES &&
		invalidCount.value === 0,
);

const submitLabel = computed(() => {
	if (isSubmitting.value) return "Adding...";
	const n = parsedLines.value.length;
	if (n === 0) return "Add Sites";
	return n === 1 ? "Add 1 Site" : `Add ${n} Sites`;
});

watch(
	() => props.visible,
	(newVal) => {
		if (!newVal) return;
		input.value = "";
		serverId.value = "";
		provider.value = "manual";
		environment.value = "production";
		status.value = "active";
		connectionType.value = "ssh";
		sshHost.value = "";
		sshPort.value = 22;
		sshUser.value = "";
		sitePath.value = "files";
		isWordpress.value = true;
		canWPCLI.value = true;
		hasAgent.value = false;
		errorMessage.value = null;
		submitErrors.value = new Map();
	},
);

function sshLabel(line: ParsedLine): string {
	const host = line.sshHost ?? sshHost.value.trim();
	if (!host) return "—";
	const user = line.sshUser ?? sshUser.value.trim();
	const port =
		line.sshPort ?? (sshPort.value === "" ? 22 : Number(sshPort.value));
	return `${user ? `${user}@` : ""}${host}${port !== 22 ? `:${port}` : ""}`;
}

function resolvedPath(line: ParsedLine): string {
	const path = line.sitePath ?? (sitePath.value.trim() || "files");
	return path.split("{domain}").join(line.domain);
}

function toPayload(line: ParsedLine): ManagedSitePayload {
	return {
		domain: line.domain,
		server_id: serverId.value,
		provider: provider.value.trim() || "manual",
		environment: environment.value,
		is_wordpress: isWordpress.value,
		connection_type: connectionType.value,
		ssh_host: line.sshHost ?? sshHost.value.trim(),
		ssh_port:
			line.sshPort ?? (sshPort.value === "" ? 22 : Number(sshPort.value)),
		ssh_user: line.sshUser ?? sshUser.value.trim(),
		site_path: resolvedPath(line),
		can_wp_cli: canWPCLI.value,
		has_agent: hasAgent.value,
		status: status.value,
	};
}

function handleOverlayClick(event: MouseEvent) {
	if (event.target === event.currentTarget) {
		emit("close");
	}
}

async function handleSubmit() {
	if (!canSubmit.value) return;

	isSubmitting.value = true;
	errorMessage.value = null;
	const lines = parsedLines.value;

	try {
		const result = await managedSitesStore.batchCreateSites(
			lines.map(toPayload),
		);
		if (result.created.length > 0) {
			emit("saved", result.created.length);
		}
		if (result.errors.length === 0) {
			emit("close");
			return;
		}

		// Keep only the lines that failed, so they can be fixed and resubmitted.
		const errors = new Map<string, string>();
		const failed: string[] = [];
		for (const e of result.errors) {
			const line = lines[e.index];
			if (!line) continue;
			errors.set(line.domain, e.error);
			failed.push(line.raw);
		}
		submitErrors.value = errors;
		input.value = failed.join("\n");
		errorMessage.value = `${result.errors.length} site(s) could not be added.`;
	} catch (e: any) {
		errorMessage.value =
			e.message || "An error occurred while adding the sites.";
	} finally {
		isSubmitting.value = false;
	}
}
</script>

<template>
	<Teleport to="body">
		<div v-if="visible" class="modal-overlay" @click="handleOverlayClick">
			<div class="modal-content card batch-site-modal">
				<header class="modal-header">
					<h2>Batch Add Sites</h2>
					<button class="modal-close" @click="emit('close')">
						<AppIcon name="x" size="20" />
					</button>
				</header>

				<div class="content">
					<div v-if="errorMessage" class="error-banner">
						<p>{{ errorMessage }}</p>
					</div>

					<form @submit.prevent="handleSubmit">
						<div class="content">
							<div class="form-group">
								<label for="batch-sites">Sites</label>
								<textarea
									id="batch-sites"
									v-model="input"
									class="batch-input"
									rows="8"
									spellcheck="false"
									autocomplete="off"
									placeholder="example.com
shop.example.com  shop@shop.ssh.wpengine.net  sites/shop
blog.example.com  deploy@10.0.0.5:2222"
								></textarea>
							</div>
							<p class="help-text">
								One site per line:
								<code>domain [user@]host[:port] [path]</code>,
								separated by spaces, tabs or commas. Leave out a
								field, or write <code>-</code>, to use the
								defaults below. Lines starting with
								<code>#</code> are ignored.
							</p>

							<div
								v-if="parsedLines.length > 0"
								class="table-container preview"
							>
								<table class="data-table">
									<thead>
										<tr>
											<th>Line</th>
											<th>Domain</th>
											<th class="hide-mobile">SSH</th>
											<th class="hide-mobile">Path</th>
										</tr>
									</thead>
									<tbody>
										<tr
											v-for="line in parsedLines"
											:key="line.line"
										>
											<td class="text-muted">
												{{ line.line }}
											</td>
											<td>
												<div
													class="font-medium text-main"
												>
													{{
														line.domain || line.raw
													}}
												</div>
												<div
													v-if="
														line.error ||
														submitErrors.has(
															line.domain,
														)
													"
													class="text-error"
												>
													{{
														line.error ??
														submitErrors.get(
															line.domain,
														)
													}}
												</div>
											</td>
											<td class="hide-mobile">
												<code>{{
													sshLabel(line)
												}}</code>
											</td>
											<td class="hide-mobile">
												<code>{{
													resolvedPath(line)
												}}</code>
											</td>
										</tr>
									</tbody>
								</table>
							</div>
							<p
								v-if="parsedLines.length > MAX_BATCH_SITES"
								class="text-error"
							>
								At most {{ MAX_BATCH_SITES }} sites can be added
								at once.
							</p>
							<p v-else-if="invalidCount > 0" class="text-error">
								Fix {{ invalidCount }} line(s) before adding.
							</p>

							<div class="section-divider">
								<h3 class="font-medium">Defaults</h3>
							</div>

							<div class="form-row">
								<div class="form-group">
									<label for="batch-server">Server</label>
									<select
										id="batch-server"
										v-model="serverId"
									>
										<option value="">None</option>
										<option
											v-for="server in serverOptions"
											:key="server.id"
											:value="server.id"
										>
											{{ server.name }}
											{{
												server.is_logical
													? "(logical)"
													: ""
											}}
										</option>
									</select>
								</div>
								<div class="form-group">
									<label for="batch-provider">Provider</label>
									<input
										id="batch-provider"
										v-model="provider"
										type="text"
										placeholder="manual, wpengine, kinsta…"
									/>
								</div>
							</div>

							<div class="form-row">
								<div class="form-group">
									<label for="batch-environment"
										>Environment</label
									>
									<select
										id="batch-environment"
										v-model="environment"
									>
										<option value="production">
											Production
										</option>
										<option value="staging">Staging</option>
										<option value="development">
											Development
										</option>
									</select>
								</div>
								<div class="form-group">
									<label for="batch-status">Status</label>
									<select id="batch-status" v-model="status">
										<option value="active">Active</option>
										<option value="paused">Paused</option>
										<option value="archived">
											Archived
										</option>
									</select>
								</div>
							</div>

							<div class="form-row">
								<div class="form-group">
									<label for="batch-connection-type"
										>Connection Type</label
									>
									<select
										id="batch-connection-type"
										v-model="connectionType"
									>
										<option value="ssh">SSH</option>
										<option value="agent">Agent</option>
										<option value="none">None</option>
									</select>
								</div>
								<div class="form-group">
									<label for="batch-ssh-host">SSH Host</label>
									<input
										id="batch-ssh-host"
										v-model="sshHost"
										type="text"
										autocomplete="off"
									/>
								</div>
							</div>

							<div class="form-row">
								<div class="form-group">
									<label for="batch-ssh-user">SSH User</label>
									<input
										id="batch-ssh-user"
										v-model="sshUser"
										type="text"
										autocomplete="off"
									/>
								</div>
								<div class="form-group">
									<label for="batch-ssh-port">SSH Port</label>
									<input
										id="batch-ssh-port"
										v-model.number="sshPort"
										type="number"
										min="1"
										max="65535"
									/>
								</div>
							</div>

							<div class="form-group">
								<label for="batch-path">Site Path</label>
								<input
									id="batch-path"
									v-model="sitePath"
									type="text"
									placeholder="files"
								/>
							</div>
							<p class="help-text">
								<code>{domain}</code> in a path is replaced with
								the site's domain, e.g.
								<code>/var/www/{domain}/public</code>.
							</p>

							<div class="section-divider">
								<h3 class="font-medium">Capabilities</h3>
							</div>

							<div class="capability-grid">
								<label class="checkbox-label">
									<input
										v-model="isWordpress"
										type="checkbox"
									/>
									WordPress site
								</label>
								<label class="checkbox-label">
									<input v-model="canWPCLI" type="checkbox" />
									WP-CLI over SSH
								</label>
								<label class="checkbox-label">
									<input v-model="hasAgent" type="checkbox" />
									jman-agent installed
								</label>
							</div>

							<div class="form-actions mt-4">
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
									{{ submitLabel }}
								</button>
							</div>
						</div>
					</form>
				</div>
			</div>
		</div>
	</Teleport>
</template>

<style scoped>
.batch-site-modal {
	max-width: 760px;
}

.batch-input {
	font-family: monospace;
	white-space: pre;
}

.preview {
	max-height: 240px;
	overflow-y: auto;

	& th,
	& td {
		padding: 6px 10px;
	}

	& code {
		word-break: break-all;
	}
}
</style>
