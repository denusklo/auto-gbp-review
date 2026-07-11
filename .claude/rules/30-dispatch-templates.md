# E. Standardized Dispatch Prompt Templates

Copy the matching template into the Agent tool `prompt`, fill every `{...}` blank,
delete nothing else. Blanks marked `(optional)` may be filled with "none".
Model choice + escalation rules: `10-model-dispatch.md` § 3.

Rules that apply to ALL templates:
- Subagents have zero conversation context. Every fact they need goes in the prompt.
- Repo root is `/home/denusklo/workspace/auto-gbp-review`. Single Go package in root.
- Tell every subagent: "Do not read `handlers.go` end-to-end; grep/codegraph first,
  then Read with offset/limit. Never print values from `.env*` files."

---

## Template 1 — Research / search  (agent type: Explore or general-purpose; model: sonnet)

```
GOAL: Find {what, precisely} in /home/denusklo/workspace/auto-gbp-review.
CONTEXT: {why this is needed — 1–2 sentences}. Known starting points: {paths/symbols/routes, or "none"}.
DO: Search with grep/codegraph. Read only the minimal excerpts needed to confirm.
DO NOT: modify any file; read handlers.go end-to-end; dump file contents.
ACCEPTANCE: Every claim carries a file:line reference you actually read.
  If not found, say NOT FOUND and list the 3+ locations/patterns you tried.
REPORT FORMAT:
  ANSWER: {≤5 sentences}
  EVIDENCE: {path:line — one-line description, per item}
  NOT FOUND / UNCERTAIN: {list or "none"}
```

---

## Template 2 — Feature implementation  (agent type: general-purpose; model: sonnet)

```
GOAL: Implement {feature} in /home/denusklo/workspace/auto-gbp-review.
CONTEXT: {background + user intent, 2–4 sentences}.
  Touch points: {files/routes/templates involved}. Follow existing patterns in
  {reference file:line of a sibling feature to imitate}.
CONSTRAINTS: Smallest working change (ponytail). No new dependencies. No schema
  changes — if you conclude one is required, STOP and report that instead of doing it.
  No edits outside: {allowed path list}.
ACCEPTANCE (all must hold):
  1. go build ./... && go vet ./...  → exit 0
  2. {behavioral check, e.g.: `go run .` then `curl -s localhost:8082/{route}`
     returns {expected}}
  3. git diff touches only the allowed paths
REPORT FORMAT:
  RESULT: {one line}
  FILES CHANGED: {path: +adds/−dels, one per line}
  VERIFIED-BY: {commands run + last ~5 output lines}
  SKIPPED/UNCERTAIN: {list or "none"}
  Do not paste full file contents — diffs of ≤30 lines only if essential.
```

---

## Template 3 — Refactoring  (agent type: general-purpose; model: sonnet)

```
GOAL: Refactor {what} — {target shape, e.g. "extract review-CRUD handlers from
  handlers.go into reviews_handlers.go, same package, zero behavior change"}.
CONTEXT: {why}. Symbols/regions in scope: {list from grep/codegraph, with lines}.
CONSTRAINTS: Behavior-preserving ONLY — no renames of exported symbols, no logic
  "improvements", no reformatting of untouched code. If a behavior change seems
  necessary, STOP and report it.
ACCEPTANCE:
  1. go build ./... && go vet ./... → exit 0
  2. {smoke check: run app + curl the affected routes → same responses as before}
  3. git diff shows moves, not rewrites (moved code textually identical except
     imports/receivers)
REPORT FORMAT:
  RESULT / FILES CHANGED / VERIFIED-BY / SKIPPED-UNCERTAIN  (as Template 2)
```

---

## Template 4 — Verification / code review  (agent type: general-purpose or
superpowers:code-reviewer; model: sonnet; MUST be a fresh agent, never the implementer)

```
ROLE: Independent verifier. You did NOT write this change; trust nothing in the
  summary below until you confirm it on disk.
CHANGE UNDER REVIEW: {one-paragraph description}. Files: {list}.
ACCEPTANCE CRITERIA TO CHECK:
  {numbered list copied verbatim from the implementation dispatch}
DO:
  1. Read the changed files from disk (offset/limit for big files).
  2. Run: go build ./... && go vet ./...
  3. Exercise the behavior: {exact commands, e.g. `go run .` + curl calls}.
  4. Scan the diff for: leftover debug prints, secrets, unrelated edits,
     special-case hacks (matrix § 1 S5).
REPORT FORMAT:
  VERDICT: PASS | FAIL
  PER-CRITERION: {n}: PASS/FAIL — {evidence: command + output tail, or file:line}
  DEFECTS: {file:line — one sentence each, most severe first; or "none"}
  Do not fix anything. Report only.
```

---

## Escalation dispatch (when Sonnet failed twice → Opus)

Prepend to the original template:

```
ESCALATION — two failed attempts by a previous agent. Original task below.
FAILURE TRACE:
  Attempt 1: {what it changed} → {why verifier failed it / error output}
  Attempt 2: {what it changed} → {why it failed}
Diagnose the root cause FIRST (state it in one sentence) before writing any code.
Consider that the task definition itself may be wrong — if so, report that instead.
```
