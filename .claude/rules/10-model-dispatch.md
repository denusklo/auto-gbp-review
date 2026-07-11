# C. Model Dispatch & Escalation Protocol

Contract for how the main-conversation model (the "commander") uses subagents in this
repo. Applies to any commander model (Opus, Sonnet). Templates to copy are in
`30-dispatch-templates.md`.

## 1. The commander does not get its hands dirty

The main conversation's context is the scarcest resource in the system. Protect it.

- **Delegate to a subagent** (Agent tool): any search across 4 or more files, any repo-wide
  scan, reading any file >500 lines, log trawling, multi-file refactors, and all
  verification passes.
- **Do directly in the main conversation:** single-file edits when you already know
  the file and line, 1–2 targeted codegraph/grep calls, running the verification gate,
  talking to the user.
- Subagent reports come back as conclusions + `file:line` pointers, never code dumps.
  If a subagent returns >100 lines of pasted code, the dispatch prompt was wrong —
  fix the prompt (use the templates), don't paste the dump onward.
- Fan out: independent subtasks go in ONE message as parallel Agent calls.

## 2. The three-piece dispatch package (mandatory in every Agent prompt)

Every subagent prompt MUST contain all three, in this order:

1. **Goal + context**: what to achieve, why, and the 3–5 facts the agent needs
   (relevant paths, the route/symbol names, constraints). Subagents start with zero
   conversation context — never write "as discussed above."
2. **Acceptance criteria**: objectively checkable conditions ("`go build ./...`
   exits 0", "returns the list of files matching X with line numbers", "endpoint
   `/api/reviews` returns 200 with JSON array"). Never "make it good."
3. **Report format**: exactly what to return. Default:
   `RESULT: <one line> / FILES: <path:line list> / VERIFIED-BY: <command + output tail>
   / OPEN QUESTIONS: <list or "none">`.

A dispatch missing any piece is a protocol violation — rewrite it before sending.

## 3. Model ladder: escalation and de-escalation

Choose the model with the Agent tool's `model` parameter (`haiku`/`sonnet`/`opus`).

| Tier | Default jobs |
|---|---|
| haiku | Mechanical batch work from an exact recipe: apply a proven pattern to N files, run commands and report output, simple greps |
| sonnet | Default worker: search/research, implementation of well-specified tasks, refactors, reviews |
| opus | Commander; called as worker only via escalation, for cross-cutting design, or gnarly debugging |

**Escalation rules (hard, not judgment calls):**
- Haiku makes ONE tool error or syntax error → immediately resend the same subtask
  to Sonnet. Do not retry Haiku.
- Sonnet fails the SAME subtask TWICE (fails acceptance criteria, or verifier rejects
  twice) → escalate to Opus, and the escalation prompt MUST include the full failure
  trace: both attempts' diffs/outputs and the verifier's rejection reasons.
- Any single subtask gets at most TWO retry rounds total across all tiers. After
  that: stop, check `20-judgment-matrix.md` § 1 (wrong-direction signals) — the task
  definition is probably wrong, not the worker.

**De-escalation rule:** when a higher tier solves something that is actually a
repeating pattern (e.g., fixes one handler; nine siblings need the identical change),
extract the recipe as an exact before/after example and dispatch the remaining
instances to Haiku/Sonnet in parallel. The recipe must be concrete enough that the
worker makes zero decisions.

## 4. Isolated verification (no self-grading)

**The agent that wrote the change never judges whether the change is done.**

- After implementation, the commander dispatches a **fresh-context verifier**
  (separate Agent call, template 4 in `30-dispatch-templates.md`). The verifier:
  reads the changed files from disk (not from the implementer's report), runs
  `go build ./... && go vet ./...`, exercises the changed behavior (run the app,
  curl the route, or run the relevant test), and returns PASS/FAIL per acceptance
  criterion with evidence.
- The commander relays FAIL verdicts back to the implementer (or escalates per § 3);
  it does not "eyeball-override" a FAIL.
- For taste-adjacent work (template wording, UI layout) where PASS/FAIL is undefined,
  verification cannot save you — follow `20-judgment-matrix.md` § 4 instead.
- Small exception: single-line mechanical changes (typo, constant) need only the
  build+vet gate run by the commander, not a separate verifier.
