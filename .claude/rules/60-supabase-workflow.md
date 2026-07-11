# Supabase & Database Workflow

Extracted from the pre-2026-07-11 CLAUDE.md, corrected and trimmed. Read this before
any migration, schema, or Supabase-touching task.

## Golden rules

1. **All development happens against the LOCAL Supabase stack** (`supabase start`,
   Studio at http://localhost:54323). Production actions require explicit user
   instruction in the current conversation (see CLAUDE.md § Hard safety rules).
2. Migrations are managed by the Supabase CLI only. `database.go`'s `migrate()`
   still executes on every app startup but is frozen legacy code — never add new
   migration logic to it.
3. Never run `supabase db reset` without telling the user first — it deletes all
   local data.
4. Never echo, log, or commit secrets (`.env*`, DB passwords, `sbp_` tokens).

## Local development loop

```bash
supabase start                                # start local stack
supabase db push                              # apply migrations to local DB
# make schema changes in Studio or SQL, then:
supabase db diff --schema public -f descriptive_name.sql   # generate migration
supabase db push                              # apply and test locally
supabase migration list                       # check status
```

Migration files live in `supabase/migrations/`. Use descriptive names; the CLI adds
the timestamp prefix. Test locally before committing the migration file.

## Production deployment (USER-INSTRUCTED ONLY)

```bash
supabase link --project-ref <PROD_REF>   # link once
supabase db push                          # applies pending committed migrations
```

Before pushing to production: (a) migration was applied and tested locally,
(b) the user approved in this conversation, (c) you stated which migration files
will run. Prefer backward-compatible changes; back up before major schema changes.

## MCP tools (`mcp__supabase__*`)

- Read tools (`list_tables`, `get_logs`, `get_advisors`, `search_docs`) — free to use;
  start debugging with `get_logs` + `get_advisors`.
- Write tools (`execute_sql`, `apply_migration`) hit the **remote linked project**
  via a full-access token — they are in the permission `ask` list and additionally
  require explicit user instruction. Prefer the local CLI equivalents.

## Schema facts (verified 2026-07-11)

- Auth uses Supabase Auth's `auth.users`; roles in user metadata + `user_roles` table.
- The public `users` table exists but is **empty/legacy** — do not read or write it.
