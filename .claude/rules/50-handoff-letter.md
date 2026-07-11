# G. Handoff Letter to Future Sessions

From Fable 5, founding session, 2026-07-11. You (probably Sonnet or Opus) are now
responsible for this environment. Everything else in `.claude/rules/` is procedure;
this file is perspective.

## 1. Three things the user did not ask about, but matter most

### 1.1 Secrets are currently the biggest real-world risk — two need human rotation
The pre-fix `settings.local.json` carried a Supabase DB password in plaintext (now
removed; backup `.bak` still contains it — that's intentional per backup policy, but
it means the password must be treated as burned). Additionally, `~/.claude.json`
registers the Supabase MCP server with a full-access `sbp_` token, no `--read-only`,
no `--project-ref` scoping — every session can write to every Supabase project on
the account. **PENDING HUMAN ACTIONS (remind the user until done, then delete this
paragraph):** (a) rotate the Supabase DB password; (b) rotate the `sbp_` token and
re-register the MCP server with `--read-only --project-ref <ref>`, granting write
via CLI only when needed; (c) delete `settings.local.json.bak` after rotation.

### 1.2 The absence of tests makes every other rule weaker than it looks
Isolated verification (10-model-dispatch § 4) currently means "build, vet, curl."
That catches maybe half of regressions in a handlers.go this size. The single
highest-leverage improvement any future session can make: when you touch a handler,
leave behind one `httptest`-based test for it (Go stdlib only, no framework — ponytail
compliant). Twenty sessions of that habit produces a real safety net for free.
Don't do a big-bang test-writing project; accrete.

### 1.3 This app has real merchants on the public internet
`/merchant?bn=` is a public, unauthenticated page; admin/merchant dashboards gate on
role metadata. Two standing implications: (a) any change to `SupabaseAuthMiddleware`
or role checks is security-critical — always dispatch a fresh verifier that actually
attempts an unauthorized request; (b) anything rendered into templates from DB values
is an XSS surface — Go's `html/template` auto-escaping protects you ONLY if nobody
"fixes" an escaping problem with `template.HTML(...)`. Treat any diff introducing
`template.HTML` as a circuit-breaker event (matrix § 3 C1-adjacent).

## 2. How this system will decay, and the countermeasures

| Decay mode | What it looks like | Countermeasure |
|---|---|---|
| Rule inflation | Every incident adds a paragraph; CLAUDE.md creeps back to 7 KB; models stop reading any of it | Hard length discipline: 40-knowledge-iteration § 3. Lessons go to LESSONS.md, not CLAUDE.md |
| Permission re-bloat | Weak models re-approve `rm:*`, `git push:*` into `allow` "to reduce prompts" | settings changes require user consent (40-… § 1). If prompted often, propose narrow allows (`rm tmp/*`), never wildcards |
| Reality drift | Code evolves; rules still say "handlers.go ~1,900 lines" or cite dead routes; models learn the docs lie and ignore them | Any model that catches a stale fact fixes it immediately (allowed without asking) — trust in the docs is the asset being protected |
| Verification theater | Verifier dispatches degrade into "looks good, PASS" without running anything | The PER-CRITERION report format requires command output as evidence; a PASS without pasted evidence is a FAIL — commanders must bounce it |
| Backup litter | `.bak` files accumulate and confuse searches | Delete a `.bak` once its change is committed to git (backups exist for the gap before commit) |
| Doctrine capture | A future plugin/skill with a loud prompt overrides this harness by shouting | The precedence ladder in CLAUDE.md is the constitution; new plugins slot in at level 4 or below unless the user says otherwise |

## 3. Unfinished items from the founding session

- Global-scope issues were out of bounds ("only touch this repo"): the `~/.claude.json`
  MCP token (§ 1.1), global `model: fable` setting (stale after this session — user
  should set their preferred daily driver), and the always-on ponytail/superpowers
  SessionStart injections (token overhead every session; user may want ponytail
  scoped per-project).
- No CI. If this repo ever gets a remote pipeline, wire `go build && go vet && go test`
  as the gate and reference it from CLAUDE.md.
- The seven historical `*.md` status files in repo root were flagged, not deleted
  (user's call). If the user approves cleanup, archive them to `docs/history/`.
