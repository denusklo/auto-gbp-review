# Local auth browser smoke

`auth_smoke.py` drives the real visible registration, login, logout, forgot-password,
and reset-password forms. It locates controls through the accessibility tree,
clicks their coordinates, inserts text with CDP `Input.insertText`, and polls the
new document after each submission. If a background-tab click hits the correct
visible input but cannot focus it, a verified hit test permits native CDP
`DOM.focus` as a fallback. It verifies dashboard access, logout protection,
Mailpit recovery delivery, the recovery callback, password update, new-password
login, and rejection of the old password.

## Prerequisites

- Run from the repository root. Installed `browser-use`, Python standard library, and
  Supabase CLI suffice. No new dependencies are needed.
- First start the application built from the current checkout at exactly
  `http://127.0.0.1:3000`. This runner never starts the app or Supabase.
- Local Supabase must already be healthy: API `127.0.0.1:54321`, PostgreSQL
  `127.0.0.1:54322`, Mailpit `127.0.0.1:54324`. Local GoTrue SMTP must deliver only
  to that Mailpit instance, not external SMTP. Allow the application's
  `http://127.0.0.1:3000/auth/callback` recovery redirect. Email confirmations may
  remain disabled; the registration UI still shows its success message.
- **Startup must explicitly set `DATABASE_URL` to a loopback PostgreSQL connection
  at `127.0.0.1:54322`, with `sslmode=disable`. Never rely on the production fallback.**
  Set `SUPABASE_URL=http://127.0.0.1:54321` and provide only local keys through
  `SUPABASE_ANON_KEY`, `SUPABASE_SERVICE_ROLE_KEY`, and any required
  `SUPABASE_SERVICE_KEY`. Do not print keys or put them in this directory.
- Explicitly set `GOOGLE_CLIENT_ID=''`, `FACEBOOK_APP_ID=''`, and `RENDER=false` in
  the app's environment. Merely unsetting variables is insufficient because the
  app loads `.env`. The scheduler has no disable flag and runs after 30 seconds;
  verify the local DB has no active `api_connections` before startup. Startup
  performs legacy schema DDL. Use local-only process egress controls if available.
- A local Chromium CDP endpoint must already exist at `127.0.0.1:9223`. Use the
  dedicated daemon below, **not `browser-use-local`**, whose launcher forces the
  shared daemon on the original development machine. The dedicated daemon avoids
  interference with concurrent tasks; do not stop or restart a shared browser.
- No other app-origin tab or app auth cookies may exist in this shared Chromium
  profile. The runner refuses either condition rather than clearing another
  task's session. It activates its own test tab, making it visible for reliable
  native coordinate input. It never activates or closes unrelated tabs.

## Run

```sh
# First change directory to <repo-root>.
BU_NAME=gbp-auth-e2e BU_CDP_URL=http://127.0.0.1:9223 BH_RECORD=0 BH_TAB_MARKER=0 \
  BH_AGENT_WORKSPACE="$PWD/tests/e2e" browser-use < tests/e2e/auth_smoke.py
```

Do not invoke the Python file directly: browser-use supplies the CDP helpers.
The command selects this credential-free directory as the harness workspace.
Do not add `.env` files here. The runner does not read application credential
files or source an environment file.

The script captures `supabase status -o json` in memory, verifies the exact local
endpoints, and never prints its output. HTTP requests bypass proxies and reject
redirects. Before browser navigation, a task-session CDP Fetch guard blocks every
request outside the three exact local origins, including CDN scripts/styles.
Offlocal document redirects fail the test. Credentials, response bodies, inbox
contents, and token-bearing URLs/fragments are never printed. Recording is off.

## Ownership and cleanup

Each run generates a unique disposable `example.com` email and two random passwords
in memory. It marks ownership before registration submission. Even if registration
fails before a user ID is available, `finally` queries local admin users, matches
only that exact email, rechecks the user by UUID, deletes that user, and verifies
absence. Cleanup rediscovers exact-recipient Mailpit envelopes even if recovery
failed before message discovery. It verifies recipients before body reads and
before deleting each exact owned ID, accepts empty or plain-text 2xx deletion
acknowledgments, verifies HTTP 404, and rescans envelopes to establish no matching
mail remains. Incomplete inspection is a cleanup failure, not a pass.
It clears only app auth cookie names on the local app domain and closes only its
newly created task tab. Cleanup failure or any failed flow exits nonzero. Output
contains only fixed stage names and PASS/FAIL summaries.

## Limits

- This is a local disposable-account test, not production coverage. It does not
  test OAuth, email-confirmation signup, merchant creation, admin roles, or posting.
- Mailpit must expose `/api/v1/info`, `/api/v1/messages`, and `/api/v1/message/{id}`.
  Recovery discovery examines the newest 100 messages. Cleanup scans at most five
  100-envelope pages and deletes at most 20 exact-recipient messages. A full fifth
  page, broken pagination, missing recipient metadata, or matching leftovers
  fails cleanup because absence cannot be established. Unrelated bodies are never
  read or deleted. Recovery links must target the exact local API and callback.
- External assets are blocked, so the test checks native form behavior rather than
  styling. Native HTML password validation remains active.
- Forced process termination, a dead browser, or an unavailable local API can prevent
  `finally` cleanup. Do not use SIGKILL. Investigate any cleanup failure;
  never compensate by deleting unrelated users, messages, or all browser cookies.
- Syntax checks do not establish browser acceptance. Execute against the current
  checkout's server, then obtain independent review before accepting changes.

Offline cleanup tests, including failures before mail discovery:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests/e2e -p 'test_*.py'
```

Offline syntax check without bytecode files:

```sh
python3 - <<'PY'
from pathlib import Path
compile(Path('tests/e2e/auth_smoke.py').read_text(), 'auth_smoke.py', 'exec')
print('Python syntax OK')
PY
```
