"""Run through browser-use stdin, not directly with Python. Never log secrets."""
import html.parser
import json
import os
import secrets
import subprocess
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

APP = "http://127.0.0.1:3000"
API = "http://127.0.0.1:54321"
INBOX = "http://127.0.0.1:54324"
ORIGINS = {APP, API, INBOX}
MARKER = "gbp-auth-e2e"
COOKIE_NAMES = {"sb_access_token", "sb_refresh_token", "reset_access_token", "auth_token",
                "reset_csrf", "recovery_bridge_csrf"}
email = "gbp-auth-e2e-" + uuid.uuid4().hex + "@example.com"
old_password = secrets.token_urlsafe(32)
new_password = secrets.token_urlsafe(32)
admin_key = None
target = None
session = None
created_tab = False
registration_attempted = False
owned_messages = set()
stop_guard = threading.Event()
guard_errors = []
guard_thread = None
stage = "preflight"


class SmokeFailure(RuntimeError):
    """Only fixed source-code labels may be supplied, never external text."""


def require(condition, label):
    if not condition:
        raise SmokeFailure(label)


def origin(url):
    p = urllib.parse.urlsplit(url)
    require(not p.username and not p.password, "URL credentials forbidden")
    return p.scheme + "://" + p.netloc


def local_url(url):
    require(origin(url) in ORIGINS, "nonlocal URL forbidden")
    return url


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise RuntimeError("HTTP redirect forbidden")


opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())


def request(url, method="GET", body=None, admin=False):
    local_url(url)
    headers = {"Content-Type": "application/json"}
    if admin:
        require(origin(url) == API and admin_key, "admin endpoint invalid")
        headers.update({"apikey": admin_key, "Authorization": "Bearer " + admin_key})
    data = json.dumps(body).encode() if body is not None else None
    with opener.open(urllib.request.Request(url, data=data, headers=headers, method=method), timeout=10) as response:
        require(200 <= response.status < 300, "HTTP success status required")
        raw = response.read(2 * 1024 * 1024 + 1)
        require(len(raw) <= 2 * 1024 * 1024, "response too large")
        if method == "DELETE":
            # Mailpit may acknowledge deletion with text/plain rather than JSON.
            # Check status here; cleanup separately verifies exact-ID absence.
            return None
        return json.loads(raw) if raw.strip() else None


def load_local_status():
    # Do not inherit remote Supabase overrides, proxy settings, or Docker targets.
    env = {k: v for k, v in os.environ.items() if not k.upper().startswith("SUPABASE_")
           and not k.upper().endswith("PROXY") and k not in {"DOCKER_HOST", "DOCKER_CONTEXT"}}
    result = subprocess.run(["supabase", "status", "-o", "json"], capture_output=True,
                            env=env, timeout=20, check=False)
    require(result.returncode == 0, "local status failed")
    status = json.loads(result.stdout)
    require(status.get("API_URL") == API, "local API mismatch")
    require(status.get("MAILPIT_URL", status.get("INBUCKET_URL")) == INBOX, "local inbox mismatch")
    db = urllib.parse.urlsplit(status.get("DB_URL", ""))
    require(db.hostname == "127.0.0.1" and db.port == 54322, "local database mismatch")
    key = status.get("SERVICE_ROLE_KEY")
    require(isinstance(key, str) and bool(key), "local admin key missing")
    return key


def network_guard():
    # Fetch pauses requests before transmission, including redirects and CDN assets.
    # The dedicated daemon keeps its event buffer isolated from concurrent tasks.
    try:
        while not stop_guard.is_set():
            for event in drain_events():
                if event.get("method") != "Fetch.requestPaused" or event.get("session_id") != session:
                    continue
                p = event["params"]
                try:
                    allowed = origin(p["request"]["url"]) in ORIGINS
                except Exception:
                    allowed = False
                if allowed:
                    cdp("Fetch.continueRequest", session_id=session, requestId=p["requestId"])
                else:
                    cdp("Fetch.failRequest", session_id=session, requestId=p["requestId"], errorReason="BlockedByClient")
                    if p.get("resourceType") == "Document":
                        guard_errors.append("offlocal navigation blocked")
            stop_guard.wait(0.03)
    except Exception:
        guard_errors.append("network guard failed")


def poll(predicate, label, timeout=20):
    deadline = time.monotonic() + timeout
    time.sleep(0.25)  # wait_for_load alone can observe the pre-click document.
    while time.monotonic() < deadline:
        require(not guard_errors, "network guard failure")
        require(current_tab()["targetId"] == target, "task tab changed")
        try:
            if predicate():
                return
        except RuntimeError:
            raise
        except Exception:
            pass  # execution context may disappear during a legitimate navigation.
        time.sleep(0.15)
    # Emit only fixed structural flags, never text, query strings, or tokens.
    if target and current_tab()["targetId"] == target:
        print("CHECK", json.dumps(js("({local:location.origin === 'http://127.0.0.1:3000',login:location.pathname === '/login',dashboard:location.pathname === '/dashboard/',home:location.pathname === '/',reset:location.pathname === '/reset-password',logoutForm:!!document.querySelector('form[action=\"/logout\"]'),dashboardHeading:document.body.innerText.includes('Merchant Dashboard'),invalidCredentials:document.body.innerText.includes('Invalid credentials'),templateError:document.body.innerText.includes('Template parsing error'),forbidden:document.body.innerText.includes('Forbidden')})")))
    raise SmokeFailure(label)


def assert_app():
    require(current_tab()["targetId"] == target, "wrong task tab")
    require(js("location.origin") == APP, "wrong form origin")
    require(not guard_errors, "network guard failure")


def navigate(path, predicate):
    local_url(APP + path)
    goto_url(APP + path)
    poll(lambda: js("document.readyState === 'complete'") and predicate(), "navigation did not settle")
    assert_app()


def path_is(path):
    return js("location.origin === " + json.dumps(APP) + " && location.pathname === " + json.dumps(path))


def form_ready(action):
    return js("!!document.querySelector(" + json.dumps('form[action="' + action + '"]') + ")")


def ax_backend(field=None, button=None):
    assert_app()
    for node in cdp("Accessibility.getFullAXTree")["nodes"]:
        backend = node.get("backendDOMNodeId")
        if not backend:
            continue
        if button is not None:
            if node.get("role", {}).get("value") == "button" and node.get("name", {}).get("value", "").strip() == button:
                return backend
        else:
            attrs = cdp("DOM.describeNode", backendNodeId=backend)["node"].get("attributes", [])
            attrs = dict(zip(attrs[::2], attrs[1::2]))
            if attrs.get("id") == field:
                return backend
    raise RuntimeError("visible AX control missing")


def click_backend(backend):
    assert_app()
    cdp("DOM.scrollIntoViewIfNeeded", backendNodeId=backend)
    q = cdp("DOM.getBoxModel", backendNodeId=backend)["model"]["content"]
    x, y = sum(q[::2]) / 4, sum(q[1::2]) / 4
    require(x >= 0 and y >= 0, "control outside viewport")
    click_at_xy(x, y)


def fill(field, value):
    assert_app()  # never type generated credentials into another origin.
    backend = ax_backend(field=field)
    click_backend(backend)
    focused = lambda: js("document.activeElement.id === " + json.dumps(field))
    try:
        poll(focused, "input focus acknowledgement timed out", timeout=3)
    except SmokeFailure:
        # Background Chromium can hit the visible input yet leave activeElement
        # on BODY. Diagnose that exact case before using native CDP DOM.focus.
        q = cdp("DOM.getBoxModel", backendNodeId=backend)["model"]["content"]
        x, y = sum(q[::2]) / 4, sum(q[1::2]) / 4
        require(js("document.elementFromPoint(" + str(x) + "," + str(y) + ")?.id === " + json.dumps(field)),
                "coordinate hit test does not match input")
        assert_app()
        cdp("DOM.focus", backendNodeId=backend)
        poll(focused, "native DOM focus acknowledgement timed out", timeout=3)
    cdp("Input.dispatchKeyEvent", type="keyDown", key="a", code="KeyA", modifiers=2, windowsVirtualKeyCode=65)
    cdp("Input.dispatchKeyEvent", type="keyUp", key="a", code="KeyA", modifiers=2, windowsVirtualKeyCode=65)
    cdp("Input.insertText", text=value)
    poll(lambda: js("document.getElementById(" + json.dumps(field) + ").value === " + json.dumps(value)),
         "input insertion acknowledgement timed out", timeout=3)


def submit(button, predicate):
    assert_app()
    require(js("Array.from(document.forms).every(f => new URL(f.action).origin === location.origin)"), "external form action")
    stamp = uuid.uuid4().hex
    js("document.documentElement.dataset.e2eDocument = " + json.dumps(stamp))
    click_backend(ax_backend(button=button))
    poll(lambda: js("document.documentElement.dataset.e2eDocument !== " + json.dumps(stamp))
         and js("document.readyState === 'complete'") and predicate(), "submission did not settle")
    assert_app()


def owned_users():
    found = []
    for page in range(1, 101):
        query = urllib.parse.urlencode({"filter": email, "page": page, "per_page": 100})
        data = request(API + "/auth/v1/admin/users?" + query, admin=True)
        users = data.get("users", [])
        found.extend(u for u in users if u.get("email") == email)
        if len(users) < 100:
            return found
    raise RuntimeError("owned account lookup incomplete")


def login(password, accepted):
    navigate("/login", lambda: path_is("/login") and form_ready("/login"))
    fill("email", email)
    fill("password", password)
    if accepted:
        submit("Sign in", lambda: path_is("/dashboard/") and form_ready("/logout")
               and js("document.body.innerText.includes('Merchant Dashboard')"))
    else:
        submit("Sign in", lambda: path_is("/login") and form_ready("/login")
               and js("document.body.innerText.includes('Invalid credentials')"))


def logout():
    submit("Logout", lambda: path_is("/"))
    navigate("/dashboard", lambda: path_is("/login") and form_ready("/login"))


class Links(html.parser.HTMLParser):
    def __init__(self):
        super().__init__()
        self.urls = []

    def handle_starttag(self, tag, attrs):
        if tag == "a":
            self.urls.extend(value for name, value in attrs if name == "href" and value)


def message_owned(message):
    return any(r.get("Address", "").lower() == email for r in message.get("To", []))


def owned_mail_ids():
    """Inspect bounded envelope pages only; never read unrelated message bodies."""
    matched, seen = set(), set()
    for start in range(0, 500, 100):
        data = request(INBOX + "/api/v1/messages?" + urllib.parse.urlencode({"limit": 100, "start": start}))
        require(isinstance(data, dict) and isinstance(data.get("messages"), list),
                "inbox envelope listing incomplete")
        require(data.get("start", start) == start and len(data["messages"]) <= 100,
                "inbox pagination not honored")
        for summary in data["messages"]:
            require(isinstance(summary, dict) and isinstance(summary.get("To"), list)
                    and all(isinstance(r, dict) and isinstance(r.get("Address"), str) for r in summary["To"]),
                    "inbox recipient envelopes incomplete")
            mid = summary.get("ID")
            require(isinstance(mid, str) and mid and mid not in seen, "inbox pagination duplicated IDs")
            seen.add(mid)
            if message_owned(summary):
                matched.add(mid)
        if len(data["messages"]) < 100:
            return matched
    raise SmokeFailure("inbox capacity exceeds 500-envelope cleanup bound")


def recovery_link():
    # Read only messages addressed to this run; never dump inbox content.
    data = request(INBOX + "/api/v1/messages?limit=100")
    for summary in data.get("messages", []):
        if not message_owned(summary):
            continue
        mid = summary["ID"]
        detail = request(INBOX + "/api/v1/message/" + urllib.parse.quote(mid, safe=""))
        require(message_owned(detail), "mail recipient mismatch")
        owned_messages.add(mid)
        parser = Links()
        parser.feed(detail.get("HTML", ""))
        for url in parser.urls:
            p = urllib.parse.urlsplit(url)
            if p.scheme != "http" or p.netloc != "127.0.0.1:54321" or p.path != "/auth/v1/verify":
                continue
            query = urllib.parse.parse_qs(p.query)
            if query.get("type") != ["recovery"]:
                continue
            redirect = query.get("redirect_to", [])
            require(len(redirect) == 1 and origin(redirect[0]) == APP
                    and urllib.parse.urlsplit(redirect[0]).path == "/auth/callback", "recovery redirect mismatch")
            return local_url(url)
    return None


def cleanup():
    failures = []
    if registration_attempted and admin_key:
        try:
            for user in owned_users():
                uid = str(uuid.UUID(user["id"]))
                endpoint = API + "/auth/v1/admin/users/" + uid
                verified = request(endpoint, admin=True)
                require(verified.get("email") == email, "cleanup ownership mismatch")
                request(endpoint, method="DELETE", admin=True)
            require(not owned_users(), "owned account remains")
        except Exception:
            failures.append("owned user cleanup failed")
    if registration_attempted or owned_messages:
        try:
            # A recovery request can succeed before discovery fails. Rediscover
            # exact-recipient envelopes even when owned_messages is still empty.
            candidates = owned_mail_ids() | owned_messages
            require(len(candidates) <= 20, "owned mail exceeds 20-message cleanup bound")
            for mid in candidates:
                endpoint = INBOX + "/api/v1/message/" + urllib.parse.quote(mid, safe="")
                try:
                    detail = request(endpoint)
                except urllib.error.HTTPError as failure:
                    require(failure.code == 404, "owned mail lookup requires HTTP 404 or success")
                    continue  # already absent; ownership came from a checked envelope.
                require(message_owned(detail), "cleanup mail ownership mismatch")
                request(INBOX + "/api/v1/messages", method="DELETE", body={"IDs": [mid]})
                try:
                    request(endpoint)
                except urllib.error.HTTPError as failure:
                    require(failure.code == 404, "owned mail absence requires HTTP 404")
                else:
                    raise SmokeFailure("owned mail remains after deletion")
            require(not owned_mail_ids(), "exact-recipient mail remains after cleanup")
        except SmokeFailure as failure:
            failures.append("owned mail cleanup incomplete: " + str(failure))
        except Exception:
            failures.append("owned mail cleanup failed")
    if target:
        try:
            switch_tab(target)
            cookies = cdp("Network.getCookies", urls=[APP])["cookies"]
            for cookie in cookies:
                if cookie["name"] in COOKIE_NAMES:
                    cdp("Network.deleteCookies", name=cookie["name"], domain=cookie["domain"], path=cookie["path"])
            require(not any(c["name"] in COOKIE_NAMES for c in cdp("Network.getCookies", urls=[APP])["cookies"]),
                    "local auth cookies remain")
        except Exception:
            failures.append("local cookie cleanup failed")
        stop_guard.set()
        if guard_thread:
            guard_thread.join(timeout=3)
        try:
            cdp("Fetch.disable", session_id=session)
            if created_tab:
                close_tab(target)
        except Exception:
            failures.append("task tab cleanup failed")
    return failures


def run():
    global admin_key, target, session, created_tab, registration_attempted, guard_thread, stage
    require(os.environ.get("BU_NAME") == MARKER and os.environ.get("BU_CDP_URL") == "http://127.0.0.1:9223"
            and os.environ.get("BH_RECORD") == "0" and os.environ.get("BH_TAB_MARKER") == "0", "browser environment mismatch")
    require(os.environ.get("BH_AGENT_WORKSPACE") == os.path.join(os.getcwd(), "tests", "e2e"),
            "use the credential-free E2E harness workspace")
    require(not os.path.exists(os.path.join(os.environ["BH_AGENT_WORKSPACE"], ".env")),
            "harness workspace environment file forbidden")
    stage = "preflight local status"
    admin_key = load_local_status()
    stage = "preflight Mailpit"
    request(INBOX + "/api/v1/info")  # fail closed if this is not Mailpit.
    stage = "preflight task tab"
    tabs = list_tabs()
    try:
        current = current_tab()
    except RuntimeError:
        # A prior run closed its owned tab; never attach to an unrelated fallback.
        current = {}
    matching = origin(current["url"]) == APP if current.get("url", "").startswith("http") else False
    reuse = matching and js("window.name === " + json.dumps(MARKER))
    # Same-origin cookies are shared across tabs. Refuse to disturb another app task.
    require(not any(t["url"].startswith(APP + "/") and (not reuse or t["targetId"] != current["targetId"])
                    for t in tabs), "another app tab is open")
    require(not any(c["name"] in COOKIE_NAMES for c in cdp("Network.getCookies", urls=[APP])["cookies"]),
            "existing app auth cookies; use a clean local browser session")
    if reuse:
        target = current["targetId"]
        session = switch_tab(target)
    else:
        target = new_tab("about:blank")  # never repurpose another task's tab.
        created_tab = True
        session = switch_tab(target)
    js("window.name = " + json.dumps(MARKER))
    cdp("Fetch.enable", session_id=session, patterns=[{"urlPattern": "*", "requestStage": "Request"}])
    guard_thread = threading.Thread(target=network_guard, daemon=True)
    guard_thread.start()
    # Background coordinate clicks demonstrably failed despite correct hit tests.
    # Show only this owned task tab so native input and submit events are delivered.
    activate_tab(target)
    stage = "registration navigation"
    navigate("/register", lambda: path_is("/register") and form_ready("/register"))
    stage = "registration email input"
    fill("email", email)
    stage = "registration password input"
    fill("password", old_password)
    stage = "registration confirmation input"
    fill("confirm_password", old_password)
    stage = "registration submit"
    registration_attempted = True  # ownership exists even if response/decode/poll fails.
    submit("Create Account", lambda: path_is("/register")
           and js("document.body.innerText.includes('Registration successful')"))
    require(len(owned_users()) == 1, "registration account missing")
    print("PASS registration")
    stage = "login and logout"
    login(old_password, True)
    logout()
    print("PASS login, dashboard, logout, protected route")
    stage = "password recovery"
    navigate("/forgot-password", lambda: path_is("/forgot-password") and form_ready("/forgot-password"))
    fill("email", email)
    submit("Send Reset Link", lambda: path_is("/forgot-password")
           and js("new URLSearchParams(location.search).get('reset_sent') === 'true'"))
    link = [None]
    poll(lambda: bool(link.__setitem__(0, recovery_link()) or link[0]), "owned recovery email missing", timeout=35)
    goto_url(link[0])
    poll(lambda: path_is("/reset-password") and form_ready("/auth/reset-password")
         and js("document.readyState === 'complete'"), "recovery callback did not reach reset form")
    assert_app()
    fill("password", new_password)
    fill("confirm_password", new_password)
    submit("Update Password", lambda: path_is("/login")
           and js("new URLSearchParams(location.search).get('password_reset') === 'true'"))
    print("PASS local email, recovery callback, password update")
    stage = "changed password login"
    login(new_password, True)
    logout()
    login(old_password, False)
    navigate("/dashboard", lambda: path_is("/login"))
    print("PASS new password accepted, old password rejected")


exit_code = 0
try:
    run()
except BaseException as failure:
    # Only a fixed category, never exception strings, URLs, or response bodies.
    category = "timeout" if isinstance(failure, TimeoutError) else "runtime" if isinstance(failure, RuntimeError) else "other"
    detail = "; check=" + str(failure) if isinstance(failure, SmokeFailure) else ""
    print("FAIL " + stage + "; category=" + category + detail)
    exit_code = 1
finally:
    try:
        cleanup_failures = cleanup()
    except BaseException:
        cleanup_failures = ["cleanup interrupted"]
    for failure in cleanup_failures:
        print("FAIL " + failure)
    if cleanup_failures:
        exit_code = 1
    elif registration_attempted:
        print("PASS exact owned account, mail, and local-cookie cleanup")
raise SystemExit(exit_code)
