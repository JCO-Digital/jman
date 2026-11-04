# AI gateway

Decided 2026-11-03. jman-api hosts AI agents that callers run through the
API. Claude does the work. The first agent, `vuln-rank`, ranks the open
vulnerability exposure groups by urgency.

## Decisions

- **Agents are defined in Go**, not edited in the UI. Prompts and schemas are
  reviewed like code. Each agent has a version, and every run stores the
  version that made it.
- **Agents are advisory (v1).** Their tools only read, and the only effect of
  a run is its row in `ai_runs`. People act on the results through jman's
  existing endpoints (update jobs, mutes, tasks).
- **Zero data retention (ZDR) is enforced,** except for batches of agents
  whose requests hold no personal data. See below.
- **Model per agent.** Agents default to `claude-opus-5-5`. The `aiAgents`
  config overrides the model, effort and schedule per agent.
- **Callers authenticate with jman's own JWTs:** web sessions and CLI tokens.
  Running an agent needs the execute level; reading runs needs basic. Scoped
  service tokens for outside systems are left for later.

## Zero data retention

ZDR is an arrangement Anthropic enables per organization; the API can't
report it. The gateway enforces its side in three places:

1. **Startup:** the gateway won't call the Claude API until `aiZdrConfirmed =
   true` says the organization of `anthropicApiKey` has ZDR.
2. **Models:** `internal/ai/pricing.go` lists the models agents may use.
   Fable and Mythos models need 30-day retention ("Covered Models") and are
   refused, as is any model not in the list (`checkModel`, at startup and on
   every run).
3. **Requests:** `checkZDR` (`internal/ai/zdr.go`) checks every request before
   it is sent. It allows only the ZDR-eligible request fields (model,
   messages, system, tools, tool choice, output config, thinking, cache
   control, stop sequences), only custom tools, and only text, tool and
   thinking content blocks.

So the gateway never uses the Files API, Managed Agents, code execution or
programmatic tool calling, the MCP connector, message threads, or cache
diagnostics.

### Batches: no personal data

The Message Batches API costs half, but it is outside ZDR: Anthropic keeps
batches for 29 days. Decided 2026-11-03: batching is fine for tasks whose
requests hold no personal data.

- **Opt-in per agent:** an agent sets `Spec.Batchable` only when its requests
  never hold personal data (people's names or contacts, site owners,
  customer details). Agents with tools can't be batched. `[aiAgents.<name>]
  batch = false` turns batching off for one agent.
- **When:** scheduled runs of a batchable agent are batched. API callers can
  ask for a batched run (`"batch": true`, `jman ai run --batch`); on-demand
  runs in the UI aren't batched.
- **Checked:** before a batch is submitted, `findPersonalData`
  (`internal/ai/pii.go`) scans the request's text. It looks for email
  addresses, phone numbers, Finnish personal identity codes, IBANs, payment
  card numbers and API keys. On a match the request is logged and sent
  through the regular ZDR API instead, and the run's `batch` is false.
- **The same request otherwise:** batches use the same field and model
  checks (`checkZDR`, `checkModel`).
- **Collected:** the run stays `running` with its `batch_id`, and the
  prompt state its agent needs to finish the answer is stored with it. A
  poller checks pending batches every minute, and a restart keeps waiting
  for them. Canceling the run cancels the batch. A batch that expires after
  24 hours fails the run.

The server-side refusal fallback (a beta) isn't used either: the ZDR
eligibility table doesn't list it, and its `default` mode picks the fallback
model itself. A refused run ends as `refused`, and `refusal_category` says
why (CVE text can trip the `cyber` classifier).

## Shape

| Part | Where |
| --- | --- |
| Agent interface, gateway, queue, schedule | `internal/ai` |
| Agents | `internal/ai/agents/<name>`, listed in `internal/ai/agents/agents.go` |
| CISA KEV and EPSS lookups (CVE IDs only) | `internal/threatintel` |
| Runs and tool calls | `ai_runs`, `ai_run_steps` in `api.db` (`internal/db/ai_run_repo.go`) |
| API | `internal/api/ai_handlers.go`, `docs/API_SPECS.md` → AI Gateway |
| CLI | `jman ai status / agents / run / runs / show / cancel` |
| Web UI | Monitoring → Vulnerabilities: "Rank with AI", AI priority badges and sort |

The CLI must not link the gateway or the Claude SDK (`cmd/jman/deps_test.go`).

### Agents

An agent implements `ai.Agent`:

- `Spec()` gives its name, version, system prompt, output JSON schema, and
  optional tools, limits, reuse window and schedule.
- `ValidateInput` checks a caller's input before the run is queued (400 when
  invalid).
- `Prompt` gathers the data and builds the first user message. Data from
  outside jman is wrapped and marked as untrusted.
- `Finalize` checks the answer and completes it (e.g. drops unknown keys,
  orders the rankings).
- `FakeAnswer` (optional, `ai.Faker`) answers without the model, for local
  development.

An agent without tools is one call. With tools it is an agentic loop (up to
`MaxTurns`) with strict tool schemas. Each response is streamed. Its final
answer is constrained by structured outputs (`output_config.format`), and its
thinking depth by `effort`; Opus 5.5 always thinks adaptively. The system
prompt is cached, and with tools, so is the conversation between turns.

### Runs

`POST /api/ai/agents/{name}/runs` queues a run. Two workers run queued runs,
and runs interrupted by a restart are queued again (agents only read). A run
ends `succeeded`, `failed`, `refused` or `canceled`. A caller may wait for the
end up to 120 seconds (`?wait=`), or poll.

- **Reuse:** with `ReuseFor`, a run whose first request is identical to one
  that succeeded within that window reuses its output (`reused_from`) instead
  of calling the model. For `vuln-rank` that is 24 hours, so repeated clicks
  and the nightly run don't pay again while nothing changed.
- **Schedule:** an agent with `ScheduleEvery` is queued by jman-api itself
  (`trigger: schedule`), checked every 15 minutes. `vuln-rank` runs daily.
- **Cost:** each response's usage is priced from the table in `pricing.go`.
  Cache writes cost 1.25 times the input price, and batched requests half. The total is stored as
  `cost_micros`. It is an estimate for the budget; the Claude Console has the
  real bill.
- **Budget:** a model call is refused once today's (UTC) estimated spend
  reaches `aiDailyBudgetUsd` (default $10). Runs going on at the same time
  can overshoot it by about one run. `MaxCostUSD` stops a single run of an
  agent with tools.

## vuln-rank

The model gets, per open exposure group (or the groups in `input.keys`):

- the name, closed status and last update
- the highest CVSS
- each vulnerability's title, CVEs, CVSS score, vector and severity, CWEs and
  description (cut at 1,500 characters, marked)
- CISA KEV listing and EPSS score per CVE
- installed and fixing versions, and days open and days with a fix available
- **counts** of affected sites: total, by environment
  (production/staging/development/unknown), fix available, no fix, overdue
  and muted

No site names, domains, IDs or mute reasons are sent, and email addresses in
feed text (researchers' contacts in advisories) are replaced with "[email
removed]". The agent is batchable, so its daily scheduled run goes through
the Batch API.

It returns one ranking per group:

- `priority` P1–P4, with score bands 80–100 / 60–79 / 30–59 / 0–29
- `score` from 0 to 100
- `action`: `update_now`, `update_this_week`, `remove_plugin`, `mute_candidate`
  or `investigate`
- a one- or two-sentence `rationale`
- `confidence`

`Finalize` drops keys that weren't sent and duplicates, clamps scores, orders
by score and lists left-out groups in `unranked`.

Roughly 50 groups is about 50k input and 10k output tokens: about $0.40 a run
on Opus 5.5 at effort high, about $0.20 for the batched daily run, and
nothing when nothing changed (reuse).

The UI shows the latest successful ranking: a priority badge on each group,
the suggestion and rationale when it is expanded, and an "AI priority" sort.
Rankings are suggestions. Overdue state, mutes and alerts don't depend on
them.

## Configuration

```toml
anthropicApiKey = "sk-ant-..."   # or JMAN_ANTHROPICAPIKEY
aiEnabled = true
aiZdrConfirmed = true            # the key's organization has ZDR
aiDailyBudgetUsd = 10

[aiAgents.vuln-rank]             # optional overrides
model = "claude-sonnet-5-5"
effort = "medium"
scheduleHours = 12               # negative turns the schedule off
batch = false                    # no Batch API for this agent
```

Locally, `make local-dev` plus `make local-dev-agent` give `vuln-rank` real
exposures: the containers' sites run old plugin releases with published CVEs
(one in CISA KEV) and an older WordPress core. The local admin has TOTP, so
"Rank with AI" and `.local-dev/jman ai run vuln-rank` work. Runs get
rule-based answers (`aiFake`) unless `LOCAL_DEV_ANTHROPIC_API_KEY` is set,
which uses the Claude API with that key. See README.md, "Local development".

## Adding an agent

1. Add `internal/ai/agents/<name>` with a type implementing `ai.Agent` (and
   `ai.Faker`), an embedded `prompt.md`, and a test.
2. List it in `agents.All()`.
3. Keep tools read-only and their results free of personal data (contacts,
   emails, payment details).
4. Set `Batchable` only when the requests never hold personal data. Keep
   `State` JSON, since a batched run stores it until its batch ends.
5. Change `Version` whenever the prompt, schemas or tools change.

## Later

- Scoped service tokens for systems outside jman.
- More agents, for example scan-finding triage (with tools over the scan
  data), incident summaries, and plugin update risk from changelogs.
- An eval set for `vuln-rank` (labelled exposure groups with expected
  priorities) to measure prompt changes before they ship.
