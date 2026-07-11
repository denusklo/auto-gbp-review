# Lessons Learned Log

Append-only pitfall log. Format defined in `40-knowledge-iteration.md`. Newest entries at top.

## 2026-07-11 | PITFALL | supabase
SYMPTOM: repo is linked to the PROD Supabase project (supabase/.temp/project-ref exists).
CAUSE: user previously developed directly against prod; local schema is BEHIND prod.
RULE: never `supabase db push` here (it targets prod). Apply migrations locally with
`supabase migration up` only. Prod schema may have objects with no migration file.
COST: none yet — caught before damage.

## 2026-07-11 | PITFALL | go
SYMPTOM: `go run .` used to silently talk to production Supabase.
CAUSE: main.go:43 loads `.env`, which held prod values; also database.go hardcodes the
prod pooler host whenever SUPABASE_URL is set (bypassed only by DATABASE_URL, added today).
RULE: `.env` now points at the LOCAL stack (prod lines commented out, same file). Before
any prod deploy concern, check `.env.production`, not `.env`. Local DB URLs need `?sslmode=disable`.
COST: none — caught before damage.
