# F. Knowledge Iteration & Reflection Protocol

How future models keep this harness alive without corrupting it.

## 1. Edit permissions by file

| Action | Allowed without asking the user? |
|---|---|
| Append a lesson entry to `LESSONS.md` | ✅ Yes — expected after every pitfall |
| Add a NEW positive/negative example to `20-judgment-matrix.md` (append under the matching section, never alter existing rules) | ✅ Yes |
| Fix a factually wrong path/command/line-count anywhere in `.claude/rules/` or CLAUDE.md (verify the fact first, note the fix in LESSONS.md) | ✅ Yes |
| Compact LESSONS.md per § 3 | ✅ Yes |
| Add a new rules file (`.claude/rules/7x-…`) for a new recurring domain | ⚠️ Ask first — propose filename + 3-line outline |
| Change any RULE: precedence ladder, escalation thresholds, Definition of Done, circuit-breaker triggers, safety rules | ❌ User consent required, every time |
| Edit `.claude/settings*.json`, `.gitignore` harness block, or delete/rename any harness file | ❌ User consent required |
| Weaken any safety rule because it is "slowing you down" | ❌ Never. Report the friction in LESSONS.md instead |

Mechanics for ANY harness file edit: `cp <file> <file>.bak` first; make the edit;
mention the edit in your end-of-turn summary. Harness files MUST live in git history —
check `git status`; if CLAUDE.md or `.claude/rules/` show as untracked/modified,
ask the user to commit them (or commit via their normal flow) before session end.

## 2. Lesson record format (LESSONS.md)

Append at the TOP of the file. One entry per pitfall or confirmed-working pattern.
Max 8 lines per entry — a lesson longer than that hides a rule that should be
proposed properly.

```
## 2026-07-15 | PITFALL | supabase
SYMPTOM: `supabase db push` failed with "migration history mismatch".
CAUSE: migration file was renamed after being applied locally.
RULE: never rename a migration file after `db push`; use `supabase migration repair`.
COST: ~40 min, 2 failed subagent rounds.
```

Types: `PITFALL` (something bit us), `PATTERN` (a proven recipe worth reusing),
`USER-DECISION` (the user chose X over Y — cite date; reuse per matrix § 4, point 4).
Tags: `supabase | go | templates | htmx | auth | harness | other`.

Before starting any non-trivial task: skim LESSONS.md headings (30 seconds) and
grep it for your task's tag.

## 3. Compaction trigger

When `LESSONS.md` exceeds **150 lines** (check: `wc -l`), compact it in the same
session that crossed the threshold:

1. `cp LESSONS.md LESSONS.md.bak`
2. Merge duplicate/near-duplicate entries; keep the clearest RULE line of each group.
3. If ≥3 entries share one root cause, replace them with ONE generalized entry and
   propose (ask the user) promoting it into the relevant rules file.
4. Delete entries obsoleted by code changes (verify obsolescence first).
5. Target ≤80 lines after compaction. Record `## <date> | PATTERN | harness —
   compacted N entries to M` as the newest entry.

The same 150→80 discipline applies if any other rules file grows >50% beyond its
founding length: propose a split or trim to the user — rule inflation is how this
harness dies (see 50-handoff-letter.md § 2).
