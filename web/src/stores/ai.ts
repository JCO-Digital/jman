import { computed, ref } from "vue";
import { defineStore } from "pinia";
import type {
	AIRun,
	AIStatus,
	AIVulnRank,
	AIVulnRanking,
	VulnExposureGroup,
} from "../types";
import { BASE_URL, apiFetch, handleErrorResponse } from "../utils/api";

const VULN_RANK = "vuln-rank";
const FINISHED = ["succeeded", "failed", "refused", "canceled"];

/** The exposure group key the vuln-rank agent uses. */
export function vulnGroupKey(g: VulnExposureGroup): string {
	return g.kind === "core" ? `core:${g.version ?? ""}` : `plugin:${g.slug}`;
}

/** jman-api's AI gateway: its status and the vuln-rank agent's ranking. */
export const useAIStore = defineStore("ai", () => {
	const status = ref<AIStatus | null>(null);
	const vulnRanking = ref<AIRun<AIVulnRanking> | null>(null);
	const ranking = ref(false);

	const vulnRanks = computed(() => {
		const byKey = new Map<string, AIVulnRank>();
		for (const r of vulnRanking.value?.output?.rankings ?? []) {
			byKey.set(r.key, r);
		}
		return byKey;
	});

	async function getJSON<T>(path: string): Promise<T> {
		const res = await apiFetch(`${BASE_URL}${path}`);
		if (!res.ok) await handleErrorResponse(res);
		return res.json();
	}

	async function fetchStatus() {
		status.value = await getJSON<AIStatus>("/ai/status");
	}

	/** Loads the latest successful vuln-rank run. */
	async function fetchVulnRanking() {
		const runs = await getJSON<AIRun<AIVulnRanking>[]>(
			`/ai/runs?agent=${VULN_RANK}&status=succeeded&limit=1`,
		);
		vulnRanking.value = runs?.[0] ?? null;
	}

	/** Runs vuln-rank, waits for it, and loads its ranking. Throws when the
	 * run doesn't succeed. */
	async function rankVulns() {
		ranking.value = true;
		try {
			const res = await apiFetch(
				`${BASE_URL}/ai/agents/${VULN_RANK}/runs?wait=60`,
				{
					method: "POST",
					headers: { "Content-Type": "application/json" },
					body: "{}",
				},
			);
			if (!res.ok) await handleErrorResponse(res);
			let run: AIRun<AIVulnRanking> = await res.json();
			const deadline = Date.now() + 15 * 60 * 1000;
			while (!FINISHED.includes(run.status)) {
				if (Date.now() > deadline) {
					throw new Error(
						"The ranking is taking long; check back later.",
					);
				}
				await new Promise((r) => setTimeout(r, 3000));
				run = await getJSON<AIRun<AIVulnRanking>>(`/ai/runs/${run.id}`);
			}
			if (run.status !== "succeeded") {
				throw new Error(run.error || `The ranking run ${run.status}.`);
			}
			vulnRanking.value = run;
			await fetchStatus();
		} finally {
			ranking.value = false;
		}
	}

	return {
		status,
		vulnRanking,
		vulnRanks,
		ranking,
		fetchStatus,
		fetchVulnRanking,
		rankVulns,
	};
});
