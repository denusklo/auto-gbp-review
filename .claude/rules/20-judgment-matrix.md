# D. Judgment Externalization Matrix

Fable 5's decision heuristics, quantified so a weaker model can compare its situation
against them mechanically. Check § 1 whenever stuck, § 2 before every "done" claim,
§ 3 before burning more autonomous turns, § 4 whenever a choice is about taste.

## 1. "Abandon this path" signals — stop patching, change approach

Retrying the same idea with small edits is the #1 weak-model failure mode. Any ONE
of these signals means: revert to the last known-good state (`git stash` /
`git checkout -- <file>`), write one sentence naming the flawed assumption, and take
a different approach or escalate.

| # | Signal (mechanical test) |
|---|---|
| S1 | Your fix for error A caused error B, and your fix for B re-caused A (a flip-flop pair within one task) |
| S2 | You are editing your OWN new code for the 3rd time while the original task is still failing |
| S3 | Making progress requires touching a file/system the task description never implied (e.g., a template-text task now "needs" an auth middleware change) |
| S4 | You cannot state in one sentence why the next edit will succeed when the previous one failed |
| S5 | You're adding special-case branches (`if x == "..."`) to force one failing input to pass |

**Perfect positive example:** Sonnet's HTMX endpoint returns 404. Attempt 1: fixes
route path — still 404. It notices the router group requires auth middleware and the
test curl has no token (S4 check: "the last fix failed because the 404 comes from
middleware, not routing — verified in logs"). New single edit passes. ✅ One retry,
each attempt driven by new evidence from logs, not by permuting the same code.

**Typical negative example:** Model gets a Go template error `undefined variable
"$biz"`, renames the variable (fail), moves the `{{define}}` block (fail), adds a
duplicate template file (fail), then edits handlers.go to pass a differently-named
struct (fail, now 2 pages broken). ❌ Four permutations, zero new evidence gathered
(never rendered the template in isolation, never read how sibling templates receive
the same data). S2 fired at edit #3 and was ignored.

## 2. Definition of Done — ALL boxes checked, or it is not done

- [ ] `go build ./...` exits 0 AND `go vet ./...` exits 0 (paste the exit evidence)
- [ ] The changed behavior was **exercised**, not inferred: app ran and the specific
      route/flow was hit (curl or browser), or a test covering the change passed
- [ ] Evidence recorded: the actual command + relevant output tail is in your report
- [ ] `git diff --stat` contains ONLY files the task required (no drive-by edits,
      no leftover debug prints, no committed binaries)
- [ ] For non-trivial logic (a branch/loop/parser/money/security path): one runnable
      check exists (per ponytail: a small `test_*.go` or assert-based check)
- [ ] Verified by a fresh-context verifier if the change spans >1 file
      (10-model-dispatch.md § 4)
- [ ] Anything skipped or uncertain is stated explicitly in the final report

**Perfect positive example:** "Done. `go build ./...` + `go vet ./...` exit 0.
Ran `go run .`, `curl -s localhost:8082/merchant?bn=test` returns 200 with the new
Waze card in HTML (output tail attached). Diff: templates/merchant.html +12/−3,
handlers.go +4. Verifier subagent: PASS on all 3 criteria. Skipped: no test added —
pure template change, no logic." ✅

**Typical negative example:** "I've implemented the Waze card and it should now
display correctly on the merchant page." ❌ No build evidence, nothing exercised,
"should" is a confession, diff not shown.

## 3. Circuit breaker — stop autonomous work and ask the user

Trigger on ANY of these. When triggered: stop, summarize state in ≤10 lines, present
the specific question with 2–3 concrete options. Do not keep "making progress" in a
direction that might be discarded.

| # | Trigger |
|---|---|
| C1 | The next action is irreversible or outward-facing: prod DB write, `git push`, data deletion, sending anything to an external service — and the user hasn't approved it in THIS conversation |
| C2 | The user's request has 2+ materially different interpretations and picking wrong wastes >30 min of work |
| C3 | Two full retry rounds exhausted (10-model-dispatch.md § 3) and the failure trace suggests the task definition — not the code — is the problem |
| C4 | You discovered mid-task that the real scope is ≥3× the requested scope (e.g., "fix this handler" actually requires a schema migration) |
| C5 | A secret, credential, or PII exposure is discovered (report immediately; never copy the value into your message) |
| C6 | A taste/product decision per § 4 |

**Positive example:** "Fixing the review-sync bug requires adding a column to
`reviews` (schema migration, C4). Options: (a) migration + backfill, ~30 min, clean;
(b) compute at read time, no migration, slower page. Which?" ✅
**Negative example:** Model silently applies the migration to the linked project via
`apply_migration` because "the fix needed it." ❌ C1 + C4 both ignored.

## 4. Taste-limit protocol — decisions weak models must not make alone

UI look-and-feel, user-facing copy/wording (especially anything merchants or their
customers read), feature scope ("should we also…"), and schema design tradeoffs are
**taste decisions**. The harness cannot verify them, so:

1. Never silently pick. Produce 2–3 concrete artifacts (mock HTML snippet, two copy
   variants, two schema sketches) — concrete enough that the user can choose in
   10 seconds.
2. State each option's tradeoff in ≤2 sentences.
3. Interactive session → AskUserQuestion. Background session → finish all
   non-taste work, then report the options as the deliverable and stop.
4. If the user has previously decided an identical pattern (check LESSONS.md and
   git history), reuse their decision and say you did.

**Positive example:** "Both grid cards work. Option A matches the existing Waze card
style (screenshot-equivalent HTML attached); Option B is denser, fits 6 cards per
row. A is consistent, B was hinted by your 'too much scrolling' comment. Pick one." ✅
**Negative example:** Model rewrites all merchant-page copy to "sound more
professional" as a drive-by while fixing a layout bug. ❌ Unrequested taste decision.
