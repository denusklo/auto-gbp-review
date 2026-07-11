# Meta (Facebook / Instagram / Threads) Posting Integration — Feasibility & Plan

Planning document only. No code was written or changed as part of this doc.
Access date for all web-sourced facts below: **2026-07-11**.

## 1. TL;DR feasibility verdict

- **Facebook Pages**: Feasible. Posting to Pages via the Graph API is a mature,
  well-documented flow (`pages_manage_posts`), but requires a Meta Developer App,
  Business Verification, and App Review before it works for any Page you don't
  personally administer.
- **Instagram**: Feasible, but with a catch — the account must be an Instagram
  **Business or Creator** account linked to a Facebook Page (personal IG accounts
  cannot be posted to via API at all; Instagram Basic Display API is dead since
  Dec 2024). Uses the same Facebook Developer App as above.
- **Threads**: Feasible but immature — Meta's Threads API is newer, has its own
  base URL/app product/permission set, and this repo currently has **zero**
  Threads code (no provider file exists at all, unlike Facebook/Instagram).
  Treat as higher-risk/lower-priority than FB/IG.

## 2. Do we need a Facebook app?

**Yes — non-negotiable, and it's actually just one app that covers all three
platforms.**

In plain terms: "Facebook app" here doesn't mean an app users install — it's a
developer registration at developers.facebook.com that acts as your integration's
identity with Meta. You create ONE app in the Meta Developer Portal, and inside
that single app you add three "products": Facebook Login/Pages API, Instagram
Graph API, and (optionally) the Threads API. All API calls — including posting on
behalf of a client's Facebook Page or Instagram account — must be authenticated
as this app plus a token authorizing access to that specific Page/account. There
is no way to call these APIs without one.

The good news for an agency model: you register the app once (as the agency
owner), then each merchant/client grants your app permission to their specific
Page/Instagram account via OAuth — you do not need a separate app per client.

## 3. Step-by-step setup path

1. **Create a Meta Developer account** at developers.facebook.com (free, tied to
   a personal or business Facebook account).
2. **Create one App** in the Developer Portal, choosing the **"Business"** app
   type (required for Page-posting use cases).
3. **Complete Business Verification** for the app's associated Business Manager —
   verifies your legal business entity (documents, domain verification, sometimes
   a phone call from Meta). Required before Advanced Access to permissions is
   granted. ([Meta Business Verification guide](https://agrowth.io/blogs/facebook-ads/how-to-verify-your-business-on-meta), accessed 2026-07-11)
4. **Add products** in the app dashboard: Facebook Login (for Pages), Instagram
   Graph API, and Threads API (separate product, if pursuing Threads).
5. **Request permissions** — at minimum for FB/IG posting:
   `pages_show_list`, `pages_read_engagement`, `pages_manage_posts`,
   `instagram_basic`, `instagram_content_publish`. For Threads:
   `threads_basic`, `threads_content_publish`.
   ([Permissions Reference](https://developers.facebook.com/docs/permissions/), accessed 2026-07-11)
6. **Submit for App Review** per permission: written use-case justification +
   a screencast video of the exact flow in your app, plus a hosted privacy
   policy URL. Vague justifications ("to improve UX") are the top rejection
   reason. ([Postproxy Facebook Graph API guide](https://postproxy.dev/blog/facebook-graph-api-posting-guide/), accessed 2026-07-11)
7. **Development-mode exception**: while in review, `pages_manage_posts` /
   `pages_read_engagement` / `pages_show_list` already work for admins/testers
   of the app — useful for building and demoing before Advanced Access is
   granted. ([Postproxy Facebook Graph API guide](https://postproxy.dev/blog/facebook-graph-api-posting-guide/), accessed 2026-07-11)
8. **Generate tokens** once approved: exchange a short-lived user token → a
   long-lived token → a Page access token (or provision a System User token in
   Business Manager — see § 4). Store encrypted per-connection, same pattern the
   repo already uses for Facebook/Instagram OAuth (`social_media/encryption.go`).

## 4. Token architecture — options for a multi-merchant agency

This is a design/tradeoff decision, not pure taste — token expiry behavior is an
objective operational cost, so a recommendation is given, but both options are
viable.

**Option A — Per-merchant OAuth (what the repo already does today for FB/IG reads)**
Each merchant clicks "Connect Facebook," goes through OAuth, grants your app a
Page-level token derived from their long-lived user token.
- Pros: matches the repo's existing pattern exactly (`social_media_handlers.go:93-121`,
  `social_media/facebook.go`); merchant explicitly consents each time; no
  Business Manager admin relationship required with every client.
- Cons: long-lived user tokens last ~60 days and Page tokens derived from them can
  be invalidated by the merchant changing their password or revoking access —
  meaning ongoing refresh/re-auth burden multiplied across every client, and a
  silent failure mode when a merchant's token dies without you.
  ([Meta access token guide](https://dev.to/alex97po/meta-oauth-short-lived-vs-long-lived-tokens-and-why-your-token-expires-after-1-hour-4609), accessed 2026-07-11)

**Option B — System User token via a shared Business Manager**
Each merchant's Page is added as an asset to your agency's Business Manager
(via Business Manager's "request access to a Page" flow), and a single System
User token (scoped per-Page via System User asset assignment) performs all
posting.
- Pros: System User tokens don't expire on a timer and are the pattern Meta
  documents for "programmatic, automated actions on Pages without requiring
  input from an app user or re-authentication" — i.e., built for exactly this
  agency/automation use case.
  ([Meta access token guide](https://dev.to/alex97po/meta-oauth-short-lived-vs-long-lived-tokens-and-why-your-token-expires-after-1-hour-4609), accessed 2026-07-11)
- Cons: onboarding a merchant now means asking them to accept a Business Manager
  Page-access request (an extra step beyond a simple OAuth click); still not
  literally permanent — invalidated by password changes/permission revocation on
  the merchant's side regardless of token type.

**Recommendation**: For a growing agency managing many clients long-term, Option
B (System User + Business Manager) is objectively better on the token-expiry
axis, which is the dominant operational cost once you have more than a handful
of merchants — recurring 60-day refresh cycles across N clients do not scale
gracefully. But it requires more merchant-onboarding friction upfront. Given the
repo currently only implements Option A, the smallest path forward is: keep
Option A for the initial rollout (reuses existing OAuth code), and revisit
Option B once merchant count or token-expiry pain justifies the switch.

## 5. What already exists in this repo's code

Verified by reading source directly (grep + Read with offset/limit, not
handlers.go end-to-end):

- **Only read/sync exists — no posting capability anywhere.** The
  `SocialMediaProvider` interface (`social_media/provider.go:8-30`) defines
  `GetAuthorizationURL`, `ExchangeCodeForToken`, `RefreshToken`, `FetchReviews`,
  `GetAccountInfo`, `ValidateToken` — there is no `PublishPost`/`CreatePost`
  method or anything resembling one.
- **Facebook provider** (`social_media/facebook.go`): OAuth flow + `FetchReviews`
  only; requested scope is `pages_show_list,pages_read_engagement,pages_manage_metadata`
  (`social_media/facebook.go:42`) — note this scope set is for *reading* Page
  data, not `pages_manage_posts` for posting.
- **Instagram provider** (`social_media/instagram.go`): also read-only —
  fetches media/comments/mentions (`social_media/instagram.go:268-376`), requests
  scope `instagram_basic,instagram_manage_comments,instagram_manage_insights,pages_show_list`
  (`social_media/instagram.go:44`) — again no publish scope.
- **No Threads provider file exists at all** — `social_media/` directory contains
  only `database.go`, `encryption.go`, `facebook.go`, `google_business.go`,
  `instagram.go`, `models.go`, `provider.go`, `scheduler.go`. No `threads.go`.
- **OAuth wiring**: `social_media_handlers.go:52-72` instantiates Facebook and
  Instagram providers only if `FACEBOOK_APP_ID` env var is set; both share the
  same `FACEBOOK_APP_ID`/`FACEBOOK_APP_SECRET`/`FACEBOOK_REDIRECT_URI` credentials
  (`social_media_handlers.go:53-70`).
- **Routes** (`main.go:170-191`): `/api/social-media/connect/:platform`,
  `/api/social-media/callback/:platform`, `/api/social-media/connections`,
  `/api/social-media/connections/:id/sync`, `/api/social-media/connections/:id/logs`,
  `/api/social-media/reviews` — all gated behind merchant auth middleware
  (`main.go:172`). No posting-related route exists.
- **DB schema** (`supabase/migrations/20251028163000_social_media_integration.sql`):
  `api_connections` table stores encrypted OAuth tokens per merchant+platform,
  with a `CHECK` constraint limiting `platform` to `'google_business', 'facebook',
  'instagram'` only (no `'threads'` value permitted yet); `synced_reviews` and
  `sync_logs` tables exist for the read/sync flow. No table exists for scheduled
  or published outbound posts.
- **Unrelated existing columns**: `database.go:90-94` and `handlers.go:475-476,
  855-856, 985-986` store plain merchant-profile URL fields
  (`facebook_url`, `instagram_url`, `threads_url`) used for display/linking on
  merchant pages — these are just link-out URLs, unrelated to the OAuth/posting
  API integration and require no changes for this feature.
- **Env vars already referenced** in code/`.env.example`: `FACEBOOK_APP_ID`,
  `FACEBOOK_APP_SECRET`, `FACEBOOK_REDIRECT_URI` (`.env.example:32-35`,
  `social_media_handlers.go:53-68`). No `INSTAGRAM_*`, `THREADS_*`, or
  `META_APP_*` variables exist anywhere in the codebase.

**Bottom line**: the repo has working OAuth + read-sync scaffolding for
Facebook/Instagram reviews, but posting is unimplemented for all three platforms,
and Threads has no code presence whatsoever.

## 6. Proposed integration outline (outline only — no code)

**New/changed routes** (under existing `api/social-media` group,
`main.go:170-191` area):
- `POST /api/social-media/connections/:id/posts` — create and publish a post to
  the connected Page/IG account/Threads profile.
- `GET /api/social-media/connections/:id/posts` — list post history/status for
  a connection.
- Optionally `POST /api/social-media/connections/:id/posts/:postId/retry` if
  scheduling/retry is wanted later (defer — YAGNI until proven needed).

**Provider interface change**:
- Extend `SocialMediaProvider` (`social_media/provider.go:8-30`) with a
  `PublishPost` method, implemented per-platform (Facebook, Instagram, and a new
  Threads provider file). Instagram's publish flow is container-based
  (create-container → publish-container, two calls) rather than a single POST,
  so the interface signature needs to accommodate an async/polling step for IG
  specifically.

**New DB additions**:
- Loosen the `platform` CHECK constraint in `api_connections` /
  `synced_reviews` to add `'threads'`.
- New table, e.g. `published_posts` (merchant_id, api_connection_id, platform,
  platform_post_id, content, status, error_message, created_at) — mirrors the
  existing `sync_logs` audit-trail pattern for outbound posts.

**New env vars** (names only, no values):
- `THREADS_APP_ID`, `THREADS_APP_SECRET`, `THREADS_REDIRECT_URI` (Threads is a
  separate app product with its own credentials/base URL, unlike Instagram which
  piggybacks on the Facebook app ID).
- Reuse existing `FACEBOOK_APP_ID`/`FACEBOOK_APP_SECRET`/`FACEBOOK_REDIRECT_URI`
  for both Facebook and Instagram posting (same as current read-flow pattern).
- Optionally `META_SYSTEM_USER_TOKEN` if Option B (§ 4) is adopted later.

**Scope for a first cut** (ponytail-sized): Facebook Page text/photo posting
only, reusing the existing OAuth connections merchants already created for
reading reviews (just request the additional `pages_manage_posts` scope on
reconnect). Defer Instagram and Threads publishing to follow-up work once
Facebook posting is proven end-to-end — avoids building three unproven
integrations at once.

## 7. Risks / gotchas

- **App Review takes weeks, not days.** Every meaningful posting permission
  needs review before non-admin merchants can use it; development-mode testing
  works immediately for app admins only. ([Postproxy](https://postproxy.dev/blog/facebook-graph-api-posting-guide/), accessed 2026-07-11)
- **Business Verification delays are unpredictable in practice.** Meta's stated
  estimate is ~2 business days to 14 business days, but real-world 2026 developer
  reports on Meta's own community forums describe verification stuck "In Review"
  for 14+ days, even over a month in some cases. Budget for this being the
  critical-path bottleneck, not the coding work.
  ([Meta Community Forums thread](https://communityforums.atmeta.com/discussions/Questions_Discussions/business-verification-stuck-in-review-for-14-days---advanced-access-blocked/1377080), accessed 2026-07-11)
- **Token expiry/refresh is an ongoing operational burden**, not a one-time
  setup cost — see § 4 tradeoffs; whichever option is chosen, someone/something
  needs to monitor for silently dead connections per merchant.
- **Rate limits differ per platform and are inconsistently documented even by
  Meta itself.** Instagram Content Publishing shows conflicting figures across
  current sources: 25 posts/24h (older/commonly cited) vs 100 posts/24h (Meta's
  own `content_publishing_limit` doc language, quoted secondhand). Threads
  documents a harder, clearer cap: 250 posts / 1,000 replies / 100 deletions per
  24h per profile. Check the live `/content_publishing_limit` endpoint per
  account rather than trusting a fixed number.
  ([Ayrshare](https://www.ayrshare.com/solutions/instagram-graph-api-error-9-the-25-post-daily-limit-how-to-fix-it/), [Zernio Threads API guide](https://zernio.com/blog/threads-api), accessed 2026-07-11)
- **Threads API is the newest and least battle-tested of the three** — separate
  base URL (`graph.threads.net`), separate permission set, and scopes have
  already changed once in 2026 (a new `threads_share_to_instagram` scope added
  March 25, 2026). Treat as more likely to require maintenance churn than FB/IG.
  ([Zernio Threads API guide](https://zernio.com/blog/threads-api), accessed 2026-07-11)
- **API version churn**: Graph API v18.0 and v19.0 have already expired
  (Jan/May 2026); current is v25.0 (Feb 2026), with each version supported
  roughly 2 years. Any implementation must pin a current version and plan to
  bump it periodically or calls will start failing outright.
  ([Meta Graph API v25.0 announcement](https://developers.facebook.com/blog/post/2026/02/18/introducing-graph-api-v25-and-marketing-api-v25/), accessed 2026-07-11)
- **Instagram personal accounts cannot be posted to at all** — must be
  Business/Creator, linked to a Facebook Page; Instagram Basic Display API
  (the old personal-account API) has been fully dead since December 2024.
  ([SociaVault Instagram API 2026 guide](https://sociavault.com/blog/instagram-api-deprecated-alternative-2026), accessed 2026-07-11)
- **Security note per this repo's own standing rule**: any tokens stored for
  posting must go through the same encryption pattern already used
  (`social_media/encryption.go`) — never store raw tokens, and this is a
  security-critical surface warranting a fresh verifier per this repo's
  `.claude/rules/50-handoff-letter.md` § 1.3 guidance on auth-adjacent changes.

## 8. Sources

- [Permissions Reference — Meta for Developers](https://developers.facebook.com/docs/permissions/) — accessed 2026-07-11
- [Facebook Graph API Posting: Developer Guide — Postproxy](https://postproxy.dev/blog/facebook-graph-api-posting-guide/) — accessed 2026-07-11
- [Facebook Pages API — Meta for Developers](https://developers.facebook.com/docs/pages-api/) — accessed 2026-07-11
- [Instagram Graph API Error 9: The 25-Post Daily Limit & How to Fix It — Ayrshare](https://www.ayrshare.com/solutions/instagram-graph-api-error-9-the-25-post-daily-limit-how-to-fix-it/) — accessed 2026-07-11
- [Content Publishing Limit — Instagram Platform docs](https://developers.facebook.com/docs/instagram-platform/instagram-graph-api/reference/ig-user/content_publishing_limit/) — accessed 2026-07-11
- [Threads API Documentation 2026: Complete Developer Guide — Zernio](https://zernio.com/blog/threads-api) — accessed 2026-07-11
- [Posts — Threads API — Meta for Developers](https://developers.facebook.com/docs/threads) — accessed 2026-07-11
- [Publishing — Threads API — Documentation](https://developers.facebook.com/docs/threads/reference/publishing/) — accessed 2026-07-11
- [Meta OAuth: Short-Lived vs Long-Lived Tokens — DEV Community](https://dev.to/alex97po/meta-oauth-short-lived-vs-long-lived-tokens-and-why-your-token-expires-after-1-hour-4609) — accessed 2026-07-11
- [Access Token Guide — Facebook Login — Meta for Developers](https://developers.facebook.com/docs/facebook-login/guides/access-tokens/) — accessed 2026-07-11
- [How to Verify Your Business on Meta: 2026 Guide — AGrowth.io](https://agrowth.io/blogs/facebook-ads/how-to-verify-your-business-on-meta) — accessed 2026-07-11
- [Business Verification stuck "In Review" — Meta Community Forums](https://communityforums.atmeta.com/discussions/Questions_Discussions/business-verification-stuck-in-review-for-14-days---advanced-access-blocked/1377080) — accessed 2026-07-11
- [Introducing Graph API v25.0 and Marketing API v25.0 — Meta for Developers Blog](https://developers.facebook.com/blog/post/2026/02/18/introducing-graph-api-v25-and-marketing-api-v25/) — accessed 2026-07-11
- [Instagram API Deprecated Again? What to Actually Do in 2026 — SociaVault](https://sociavault.com/blog/instagram-api-deprecated-alternative-2026) — accessed 2026-07-11
