# A. Harness Leak Diagnosis Report

Written 2026-07-11 by Fable 5 (founding session). This report is the reference base
for every other file in `.claude/rules/`. Facts here were verified against the live
environment on that date; if a fact contradicts what you observe, trust your observation
and log the discrepancy in `LESSONS.md`.

## 1. Top three leaks and their physical fixes

### Leak 1 — Instruction overload and doctrine conflict at session start
**Symptom:** Every session injects, before the user says a word: the full ponytail
prompt (SessionStart hook), the full superpowers `using-superpowers` skill text,
MCP instructions from codegraph + context7 + supabase, plus CLAUDE.md. That is
several thousand tokens of doctrine, and two of the doctrines contradict each other:
superpowers demands "brainstorm first, mandatory TDD, invoke a skill before ANY
response"; ponytail demands "shortest diff, minimal tests, no scaffolding."
A Sonnet-level model either thrashes between them (wasted turns invoking skills it
doesn't need) or freezes on which rule wins.

**Physical fix (applied):** CLAUDE.md now contains an explicit, numbered precedence
ladder (see CLAUDE.md § "Instruction precedence"). For this repo: user request >
CLAUDE.md + `.claude/rules/` > ponytail > superpowers workflow skills > defaults.
Superpowers process skills (brainstorming, TDD, writing-plans) are opt-in here, used
only when a task touches 3+ files or changes the DB schema. A weak model never has to
resolve the conflict itself — the ladder resolves it.

### Leak 2 — Unguarded destructive permission surface + secrets in config
**Symptom (pre-fix):** `.claude/settings.local.json` pre-approved `rm:*`, `kill:*`,
`pkill:*`, `git push:*`, `git filter-repo:*`, `psql:*`, `supabase db reset:*`,
`mcp__supabase__execute_sql`, and `mcp__supabase__apply_migration` — and embedded a
**plaintext production DB password** inside an allow rule
(`export SUPABASE_DB_PASSWORD=...`). Any model, at any capability level, could push
to the production database or delete files with zero human prompt. This is the
single largest way a weak model destroys value here: not bad code, but a pre-approved
destructive command run on a wrong assumption.

**Physical fix (applied):** `settings.local.json` rewritten (backup:
`settings.local.json.bak`). Secret rules removed; destructive commands moved from
`allow` to `ask`; read/build/test commands remain allowed. Remaining human actions
(cannot be done from inside this repo — see 50-handoff-letter.md):
rotate the leaked DB password, and rotate + re-scope the Supabase MCP access token
in `~/.claude.json` (currently full-access, no `--read-only`, no `--project-ref`).

### Leak 3 — The harness itself was invisible to git, and "done" was unverifiable
**Symptom (pre-fix):** `.gitignore` excluded **all** `*.md` files, `/CLAUDE.md`
explicitly, and the entire `.claude/` directory. CLAUDE.md was never version-controlled:
one bad edit by a weak model silently and irreversibly destroyed the institution, with
no diff, no history, no review. Separately, the repo has **zero automated tests**, so
"the task is complete" had no runnable meaning — models declared success after
`go build` alone, or after nothing.

**Physical fix (applied to working tree — durable only once COMMITTED; if
`git status` shows CLAUDE.md or `.claude/rules/` untracked, get them committed):**
`.gitignore` now re-includes `CLAUDE.md` and `.claude/rules/**` (backup:
`.gitignore.bak`). The standard verification command set is now defined
(CLAUDE.md § "Essential commands") and
20-judgment-matrix.md § 2 defines quantified Definition of Done. Until a real test
suite exists, minimum bar = `go build ./...` + `go vet ./...` + a live-endpoint curl
check by a fresh-context verifier (10-model-dispatch.md § 4).

## 2. Secondary findings (lower priority, still real)

- **handlers.go is 57 KB / ~1,800 lines.** A weak model that Reads it whole burns
  ~15k tokens and loses focus. Rule: locate symbols via `codegraph_search` /
  `codegraph_context` or `grep -n`, then Read with `offset`/`limit`. Never read
  handlers.go end-to-end in the main conversation.
- **Seven stale status docs in repo root** (PHASE1_MIGRATION.md, PHASE1_SUMMARY.md,
  PHASE_D_SUMMARY.md, SOCIAL_MEDIA_INTEGRATION.md, SOCIAL_MEDIA_SETUP.md,
  QUICKSTART_SOCIAL_MEDIA.md, FIXING_REDIRECTS.md). They describe
  past migrations as if current. Treat as historical records, never as current truth.
- **Committed 19 MB binaries** (`main`, `auto-gbp-review`) sit in the working tree.
  Ignore them; never Read them; never treat their behavior as current code behavior.
- **codegraph index lag:** codegraph is the preferred search tool but lags file writes
  by ~1s and can be stale after large refactors. After any multi-file rename, confirm
  with `grep` before trusting codegraph results.
- **Old CLAUDE.md contained falsehoods** ("checked into the codebase" — it wasn't;
  references to a `migrate()` function workflow that is deprecated). Rewritten.

## 3. Honesty clause — hard limits of this harness

Decomposition + isolated verification approximate high-level quality for tasks with
**checkable outcomes** (does it compile, does the endpoint return 200, does the row
appear in the table). They do **not** approximate it for:

1. **Taste/aesthetic decisions** — visual design of merchant pages, wording of
   user-facing copy, "does this UX feel right." A weak model cannot verify its way
   to good taste.
2. **Product judgment** — whether a feature should exist, whether a schema change is
   worth the migration risk.
3. **Ambiguous requirements** — when the user's request has 2+ materially different
   readings.

**Mandatory response standard when hitting these limits:** do NOT pick silently.
Produce 2–3 concrete options (screenshots/mock text/schema sketches), state the
tradeoff of each in ≤2 sentences, and ask the user via AskUserQuestion (interactive
session) or stop and report options (background session). Guessing on taste and
shipping it is a protocol violation; presenting options is the success condition.
If a factual question can't be verified by tool lookup, say "unverified" in the
report — never fabricate.
