# CLAUDE.md — auto-gbp-review

Go + Gin web app for managing Google Business Profile reviews. Server-side rendered
HTML templates + HTMX. Supabase provides Postgres, Auth (JWT), and Storage.
No frontend build step. No test suite yet (see `.claude/rules/00-diagnosis.md`).

## Instruction precedence (resolve conflicts with this ladder, top wins)

1. The user's explicit request in this conversation
2. This file and `.claude/rules/*.md`
3. Ponytail (minimal working change)
4. Superpowers process skills (brainstorming/TDD/plans) — **opt-in only**: use them
   when a task touches 3+ files or changes the DB schema; otherwise skip
5. Default system behavior

## Rules directory — read the one that matches your situation

| File | Read it when |
|---|---|
| `.claude/rules/00-diagnosis.md` | Session start in doubt; known hazards + harness limits |
| `.claude/rules/10-model-dispatch.md` | Before spawning any subagent; on any repeated failure |
| `.claude/rules/20-judgment-matrix.md` | Before declaring done; when stuck; before asking user |
| `.claude/rules/30-dispatch-templates.md` | Writing a subagent prompt (copy a template) |
| `.claude/rules/40-knowledge-iteration.md` | Before editing any harness file; after hitting a pitfall |
| `.claude/rules/50-handoff-letter.md` | Pending human actions + long-term decay risks; read once, then treat as historical |
| `.claude/rules/60-supabase-workflow.md` | Any DB/migration/Supabase work |
| `.claude/rules/LESSONS.md` | Start of any non-trivial task (skim, 1 min) |

## Architecture map (all Go files in repo root, single package)

- `main.go` — entry point, router setup
- `handlers.go` — **~1,800 lines.** Never Read whole; find symbols via codegraph or
  `grep -n`, then Read with offset/limit
- `supabase_auth.go` / `supabase_callbacks.go` — auth flows, middleware
- `database.go` — DB connection. Its `migrate()` still runs on every startup but is
  frozen legacy: never add migration logic to it — new migrations go through the
  Supabase CLI only
- `storage.go`, `supabase_client.go`, `social_media_handlers.go`, `utils/`
- `templates/` — HTML (layouts, partials, per-page); `static/` — assets
- Routes: `/` public, `/merchant?bn=` public, `/login` `/register`,
  `/admin/*` (admin role), `/dashboard/*` (merchant role), `/api/*` (HTMX)
- Roles live in Supabase Auth user metadata + `user_roles` table. The legacy
  `users` table is empty — auth is `auth.users` only.

## Essential commands

```bash
go run .                 # run app on :8082 (or: air -c .air.toml for hot reload)
go build ./... && go vet ./...   # minimum verification gate — run before claiming done
supabase start           # local stack; Studio at :54323
supabase db push         # apply migrations locally
```

Env vars: see `.env.example` — but note it says `PORT=8080` while the code default
(main.go) is **8082**; trust the code. Never print or commit values from `.env*`.

## Hard safety rules

- **Local Supabase for all development.** Anything touching production
  (`supabase link`, `git push`, `mcp__supabase__execute_sql`/`apply_migration`
  against a linked project) requires explicit user instruction in the current
  conversation — a CLAUDE.md rule or old approval does not count.
- Historical docs (`PHASE1_*.md`, `SOCIAL_MEDIA_*.md`, `FIXING_REDIRECTS.md`) and
  the committed binaries (`main`, `auto-gbp-review`) describe the **past** — never
  treat them as current truth.
- Before editing any file under `.claude/rules/` or this file, follow
  `.claude/rules/40-knowledge-iteration.md`.
