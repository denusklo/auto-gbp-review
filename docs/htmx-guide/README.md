# How htmx Works in This App

Written 2026-07-12 for auto-gbp-review. Every example below is real code from this
repo, with file references so you can follow along.

## 1. The mental model (read this first)

Traditional web page: click → browser loads a whole new page.
SPA (React etc.): click → JS calls a JSON API → JS renders HTML in the browser.

**htmx is a third way:** click → htmx makes the HTTP request for you → the **server
returns ready-made HTML** → htmx inserts that HTML into the page. No page reload,
no client-side rendering, no JSON (usually). The server stays in charge of all HTML.

Everything htmx does is controlled by `hx-*` attributes you put on HTML elements.
There is no htmx code to "write" — you only configure attributes. The library is
loaded once in `templates/layouts/base.html:28`:

```html
<script src="https://unpkg.com/htmx.org@2.0.4"></script>
```

## 2. The four questions every htmx attribute answers

| Question | Attribute | Example from this repo |
|---|---|---|
| **When** to fire? | `hx-trigger` | `hx-trigger="load"` (fire immediately when element appears) |
| **What** request? | `hx-get` / `hx-post` | `hx-post="/dashboard/profile"` |
| **Where** does the response HTML go? | `hx-target` | `hx-target="#reviews-container"` (CSS selector) |
| **How** is it inserted? | `hx-swap` | `beforeend` (append) / `innerHTML` (replace contents) / `none` (discard) |

Defaults if omitted: trigger = natural event (submit for forms, click for buttons),
target = the element itself, swap = `innerHTML`.

## 3. Real flow #1 — loading synced reviews (GET + fragment)

`templates/merchant/integrations.html:211`:

```html
<div id="synced-reviews" hx-get="/api/social-media/reviews?limit=10" hx-trigger="load">
    <p>Loading reviews...</p>
</div>
```

Step by step:

1. Browser renders the integrations page; the div appears with "Loading reviews...".
2. `hx-trigger="load"` → htmx immediately sends `GET /api/social-media/reviews?limit=10`,
   adding the header `HX-Request: true` to mark it as an htmx call.
3. Server side, `GetSyncedReviews` (social_media_handlers.go) sees that header and
   returns an **HTML fragment** — a `<ul>` of reviews, or a "No synced reviews yet"
   paragraph. (The same endpoint returns JSON when called without the header — one
   endpoint, two audiences.)
4. htmx takes the returned HTML and (default swap = `innerHTML`) replaces the
   "Loading reviews..." placeholder with it.

That's the whole feature. No JS was written.

**Rule of thumb: htmx endpoints return HTML, not JSON.** The `HX-Request` header is
how the server knows which caller it's talking to.

## 4. Real flow #2 — the profile form (POST + toast, no swap)

`templates/merchant_profile.html:49`:

```html
<form action="/dashboard/profile" method="POST" enctype="multipart/form-data"
      hx-post="/dashboard/profile"
      hx-indicator="#saving-indicator"
      hx-swap="none">
```

- `hx-post` — on submit, htmx sends the form via AJAX instead of a page reload.
  (The plain `action=` stays as a no-JS fallback.)
- `hx-indicator` — while the request is in flight, htmx shows the element
  `#saving-indicator` ("Saving profile..." spinner) by toggling a CSS class.
- `hx-swap="none"` — **discard the response body**; this form updates nothing in
  the page itself. Feedback arrives another way → next section.

## 5. Real flow #3 — toasts via HX-Trigger (server-fired events)

Problem: after saving, we want a toast notification — but toasts aren't "HTML
swapped into a spot on the page", they're a UI side effect. htmx's tool for this
is the **`HX-Trigger` response header**: the server can tell the browser
"fire this DOM event, with this data."

Server side — success (handlers.go, `UpdateMerchantProfile`):

```go
c.Header("HX-Trigger", `{"showToast":{"type":"success","title":"Profile Updated!","message":"...","icon":"fas fa-save"}}`)
c.Status(http.StatusNoContent) // 204: no body at all
```

Server side — errors (handlers.go, helper `hxErrorToast`):

```go
payload, _ := json.Marshal(gin.H{"showToast": gin.H{
    "type": "error", "title": "Error", "messages": messages, ...}})
c.Header("HX-Trigger", string(payload))
c.Status(status) // e.g. 400
```

Browser side — ONE generic listener for the whole app
(`templates/layouts/base.html`, look for `showToast`):

```js
document.body.addEventListener('showToast', function (e) {
    const d = e.detail || {};
    const messages = d.messages || [d.message || ''];
    messages.forEach(msg => (iziToast[d.type] || iziToast.info)({
        title: d.title, message: msg, icon: d.icon, timeout: d.timeout || 5000,
    }));
});
```

Full round trip:

```
[Save Profile click]
      │ htmx: POST /dashboard/profile  (HX-Request: true)
      ▼
[Go handler] validates / saves
      │ response: 204 + header HX-Trigger: {"showToast":{...}}
      ▼
[htmx] sees HX-Trigger header → fires DOM event "showToast" with that JSON as e.detail
      ▼
[base.html listener] calls iziToast → toast appears
```

Why this is the "htmx way": the **server decides** what feedback the user gets
(one line of Go per handler); the browser has one dumb, generic bridge to the
toast library. No per-page response parsing, no per-feature JS. Any future handler
gets toasts by setting one header — see `hxErrorToast` for the ready-made helper.

## 6. Real flow #4 — review templates form (POST + append)

`templates/merchant_profile.html` (Review Templates section):

```html
<form hx-post="/api/reviews/add" hx-target="#reviews-container" hx-swap="beforeend"
      hx-on::after-request="if(event.detail.successful) this.reset();">
```

- Response HTML (the new template card) is **appended** (`beforeend`) into the
  container — the list grows without reloading.
- `hx-on::after-request` is inline htmx event handling: clear the form after success.

## 7. The bug we hit, as a cautionary tale

The profile form originally had `hx-swap="afterbegin"` with no target: every
response — including *entire error pages* — got prepended into the form. Each
submit nested another full page inside the page (the "DOM grows forever" bug).
And success responses were `<script>` snippets that only executed *because* they
were swapped in — invisible coupling.

Lessons baked into the current code:

1. A form that doesn't update page content uses `hx-swap="none"`.
2. Feedback (toasts) travels via `HX-Trigger` headers, never via swapped `<script>`.
3. htmx endpoints must return either a proper HTML **fragment** meant for a
   specific target, or no body at all — never a full page, never raw JSON that
   would get pasted into the DOM.

## 8. Error handling — the global safety net

`base.html` also has:

```js
document.addEventListener('htmx:responseError', function (e) {
    // Responses that carry their own toast need no generic one
    const trigger = e.detail.xhr.getResponseHeader('HX-Trigger');
    if (trigger && trigger.indexOf('showToast') !== -1) return;
    iziToast.error({ title: 'Error', message: 'Something went wrong. Please try again.' });
});
```

Any 4xx/5xx from an htmx request shows a generic error toast — unless the response
already carries a specific `showToast` trigger, which takes precedence.

## 9. Cheat sheet for adding a new feature (project convention)

1. Put `hx-get`/`hx-post` + `hx-target` + `hx-swap` on the element. No custom JS.
2. In the Go handler: `if c.GetHeader("HX-Request") != ""` → return an HTML
   fragment (build with html/template or escaped strings — never raw DB values;
   see `GetSyncedReviews` for the pattern).
3. Toast feedback → `hxErrorToast(c, status, msgs)` or the success `HX-Trigger`
   header. Never return `<script>` tags.
4. Legacy exception: `templates/business.html` has old inline JS (`writeReview()`
   etc.). It predates this convention — feed it data if you must touch it, but
   don't copy its style for new work.
