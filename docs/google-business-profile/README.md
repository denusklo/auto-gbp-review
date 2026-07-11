# Google Business Profile API — Feasibility & Integration Plan

Status: planning doc, no code changes made. Written 2026-07-11.

## 1. TL;DR feasibility verdict

**Feasible, but gated by a manual Google approval process that must happen before
any real usage, and the agency (multi-client) model has a real policy wrinkle
that needs resolving before committing to an architecture.**

- Reading and replying to reviews, creating local posts, and updating business
  info are all possible via API today. Q&A is not (Google discontinued that API
  Nov 3, 2025).
- Every new Google Cloud project starts at **0 quota** for these APIs. You must
  submit an access-request form and wait for manual approval before the app can
  do anything beyond a trial trickle.
- The `business.manage` OAuth scope is a **restricted scope** — it requires
  Google's restricted-scope app verification and, per Google's current policy,
  an annual third-party security assessment (CASA) once the app is in
  production with real user tokens. This is the single biggest hidden cost.
- Google's policy language suggests **each client business (or your agency
  Cloud project) needs its own approved access** — reselling one project's
  approved access across many unrelated client businesses is not clearly
  sanctioned. This directly affects the OAuth architecture (§4) and needs a
  product/legal decision, not a code decision.
- This repo **already has a working OAuth + review-fetch implementation** for
  Google Business Profile (`social_media/google_business.go`) — it's real,
  compiles, and is wired into the app. It uses the deprecated-but-still-live
  v4 Reviews endpoint. This is a strong head start, not a green field.

## 2. The access-approval process (read this before writing any code)

Google does not let you just turn on these APIs and start calling them at
volume. The process, current as of 2026-07-11:

1. **Prerequisites** (per Google's own prereqs page): the Business Profile you
   want to manage must be verified and active for 60+ days, and your
   organization's website/domain should match what you're requesting access
   for. (https://developers.google.com/my-business/content/prereqs)
2. **Create a Google Cloud project**, enable the specific APIs you need
   (Account Management, Business Information, Business Profile Performance,
   Notifications, Local Posts). Enabling gives you access to the API surface
   but not meaningful quota.
3. **Submit the Business Profile API access request form**
   (https://support.google.com/business/workflow/16726127). Google asks for:
   your use case, expected call volume, whether you manage one business or
   many (agency use), and verification that your business/website is
   legitimate.
4. **Manual review by Google.** No published SLA. Secondary sources report
   anywhere from a few business days to several weeks; conflicting reports
   exist and Google does not commit to a number
   (https://legalclarity.org/how-to-complete-the-google-business-profile-api-access-request-form/
   — unverified against a Google primary source). Budget for **weeks, not
   days**, in any project timeline.
5. **Common rejection reasons** (secondary-sourced, not confirmed on a Google
   page): domain/email mismatch between the requester and the business, vague
   use-case descriptions, incomplete form fields. Treat as informed guesses,
   not gospel.
6. **OAuth verification, separately.** Because the scope
   `https://www.googleapis.com/auth/business.manage` is a *restricted scope*,
   your OAuth consent screen must pass Google's restricted-scope verification
   (brand verification ~2-3 business days, then a deeper review that can take
   additional weeks) before you can take this out of testing mode with real
   users. (https://developers.google.com/identity/protocols/oauth2/production-readiness/restricted-scope-verification)
7. **CASA security assessment.** Once in production with the restricted scope,
   Google requires an annual third-party security assessment under the App
   Defense Alliance CASA framework (tiered AL1/AL2), producing a Letter of
   Validation, renewed every 12 months.
   (https://support.google.com/cloud/answer/13465431)

**Net timeline estimate**: plan for 1-3 months of lead time across API access
approval + OAuth verification before this can go live with real client tokens,
independent of how long the Go code takes to write. This gate should be
started **immediately**, in parallel with any coding, because it's the long
pole.

## 3. API family map

The old monolithic "Google My Business API v4" was split into a federated
family of APIs around 2021-2022. As of 2026:

| API | Base host | Covers | Status |
|---|---|---|---|
| Account Management API | `mybusinessaccountmanagement.googleapis.com` | List/manage accounts, admins | Active |
| Business Information API | `mybusinessbusinessinformation.googleapis.com` | Locations, hours, attributes, categories | Active |
| Business Profile Performance API | `businessprofileperformance.googleapis.com` | Impressions, search/views metrics (replaces old Insights) | Active |
| Notifications API | part of federated suite | Pub/Sub push notifications for changes | Active |
| Local Posts API | part of federated suite | Standard/event/offer/recurring posts | Active |
| Place Actions / Verifications API | part of federated suite | Location verification, place actions | Active |
| Q&A API (`mybusinessqanda.googleapis.com`) | — | Business Q&A | **Discontinued Nov 3, 2025** — do not build on this |
| Reviews (no standalone API — still under legacy v4) | `mybusiness.googleapis.com/v4` | List reviews, reply to reviews, delete a reply | Still functional; no published sunset date, still receiving minor feature updates, but is legacy and could be deprecated with less notice than the newer federated APIs |

Key point: **there is no modern, standalone "Reviews API."** Reviews still live
under the old v4 surface (`accounts/{account}/locations/{location}/reviews`).
This repo's existing code already targets this correct-but-legacy endpoint
(see §5). You can read reviews and post/delete a reply; you **cannot** delete
or edit the review text itself via API — only Google's own UI/support process
can do that.

Sources: https://developers.google.com/my-business/ref_overview,
https://developers.google.com/my-business/content/sunset-dates

Google-discontinued pieces already gone: Insights reporting (Mar 2023, →
Performance API), Business Calls API (May 2023), Insurance/health-provider
attributes (Jul 2024), Q&A API (Nov 2025).

## 4. OAuth architecture recommendation (multi-merchant agency app)

This app manages many separate client businesses from one operator account —
the standard "agency" shape. Three architectural options, in order of
likely-correctness given Google's policy stance:

**Option A — one OAuth app (your Cloud project), each merchant grants consent
individually, tokens stored per-merchant.**
The merchant (or someone with admin rights on their GBP listing) clicks
"Connect Google Business Profile," goes through Google's OAuth consent screen,
and grants `business.manage` scope to *your* registered OAuth app. You store
their resulting access/refresh token pair, scoped to that merchant. This
matches the pattern **already implemented** in this repo (see §5) and is the
conventional shape for this kind of integration (e.g., how most GBP management
SaaS tools like Podium/Birdeye operate).
- Tradeoff: your one Cloud project's quota (300 QPM default across most of
  these APIs, per Google's limits page:
  https://developers.google.com/my-business/content/limits) is shared across
  *all* your merchants' API calls. A live 2026 forum thread shows a 31-location
  agency already hitting quota/allowlisting friction on this exact model
  (https://discuss.google.dev/t/business-profile-api-allowlisting-quota-issue-for-automated-review-replies-across-31-gbp-locations/366695).
  This is the real scaling ceiling, not code complexity.
- Google's policy page (https://developers.google.com/my-business/content/policies)
  reads as permitting "your own use" of the API but is ambiguous/restrictive
  about reselling programmatic access on behalf of unrelated third parties —
  worth a direct read (and possibly a support ticket to Google) before treating
  this as fully sanctioned. Flagging as **unverified / needs Google
  clarification**, not a blocker to prototyping.

**Option B — each client business gets its own Cloud project + own API
approval, you just hold their credentials.**
More clearly compliant with a strict reading of Google's policy, since each
client is "using their own project." Massively worse operationally — you'd be
walking every single client through the approval form in §2, which takes
weeks each, for every new client you onboard. Not viable at agency scale unless
Google's stance in Option A turns out to be disallowed.

**Option C — become a recognized "vetted partner"** with higher quota /
different terms. Referenced in scattered forum threads but **no official,
documented Business Profile Partner program was found** distinct from the
well-known Google Ads Partner program (which does not cover this API family).
Treat as unverified/does-not-exist-yet until Google documentation says
otherwise.

**Recommendation**: build Option A (matches existing code, is the industry-
standard pattern), but flag the agency-reselling policy question to the user
as a circuit-breaker item (judgment matrix C4/C6-adjacent) — get an explicit
answer from Google (support ticket, or their partner team) before scaling past
a handful of pilot merchants. Token storage: encrypted at rest, per merchant,
which is already the direction of the existing schema (§5) — no new pattern
needed.

## 5. What already exists in this repo

The Google Business Profile integration is not a green field — a working
implementation already exists as part of a generic multi-platform
"social media" sync subsystem:

- `social_media/google_business.go:13-19` — `GoogleBusinessProvider` struct
  implementing a shared `SocialMediaProvider` interface (alongside Facebook and
  Instagram providers).
- `social_media/google_business.go:37-50` — builds the OAuth authorization URL
  against `https://accounts.google.com/o/oauth2/v2/auth` with scope
  `https://www.googleapis.com/auth/business.manage`.
- `social_media/google_business.go:52-96` — exchanges an auth code for tokens
  via `https://oauth2.googleapis.com/token`.
- `social_media/google_business.go:98-139` — refresh-token flow, same token
  endpoint.
- `social_media/google_business.go:141-159` — token validation via
  `https://www.googleapis.com/oauth2/v1/tokeninfo`.
- `social_media/google_business.go:161-203` — `GetAccountInfo` calls
  `mybusinessaccountmanagement.googleapis.com/v1/accounts` (correct, current
  API).
- `social_media/google_business.go:209-326` — `FetchReviews`: gets locations
  via `mybusinessbusinessinformation.googleapis.com/v1/accounts/{id}/locations`
  (correct, current API), then for each location calls
  `https://mybusiness.googleapis.com/v4/{location}/reviews`
  (**line 254** — the legacy v4 endpoint described in §3; functional today but
  worth watching for future deprecation notices).
- `social_media/google_business.go:329-...` — `convertStarRating` maps
  Google's `"ONE".."FIVE"` string enum to a numeric rating.
- `social_media/provider.go:7-29` — the `SocialMediaProvider` interface every
  platform (including Google) must satisfy: auth URL, code exchange, refresh,
  fetch reviews, account info, validate token.
- `social_media/models.go:9-23` — `APIConnection` struct: per-merchant
  platform connection record (access/refresh token fields tagged `json:"-"` so
  they never serialize out).
- `social_media/models.go:26-42` — `SyncedReview` struct: normalized review
  record with `platform_review_id`, rating, text, reply, metadata JSON blob.
- `social_media_handlers.go:42-48` — wires up `NewGoogleBusinessProvider` using
  `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` / `GOOGLE_REDIRECT_URI` env vars,
  only if `GOOGLE_CLIENT_ID` is set.
- `social_media_handlers.go:371` — exposes `"google_business": <bool>` (is it
  configured) presumably to a settings/status view.
- `supabase/migrations/20251028163000_social_media_integration.sql:6-21` —
  `api_connections` table: `merchant_id`, `platform` (CHECK constrained to
  `google_business`/`facebook`/`instagram`), encrypted access/refresh tokens,
  `token_expires_at`, unique on `(merchant_id, platform, platform_account_id)`.
- `supabase/migrations/20251028163000_social_media_integration.sql:26-43` —
  `synced_reviews` table: per-merchant, per-platform, `platform_review_id`
  unique per platform, rating, text, reply, metadata JSONB.
- `supabase/migrations/20251028163000_social_media_integration.sql:48-63` —
  `sync_logs` table: tracks sync runs (`sync_type`, `status`, counts,
  `error_message`, timestamps).

**What's missing relative to what actually replying to a review or creating a
post requires:** no `ReplyToReview` / `CreatePost` / `UpdateBusinessInfo`
methods exist yet on `GoogleBusinessProvider` or the `SocialMediaProvider`
interface — only read (fetch reviews, account info). No route in
`social_media_handlers.go` was found that triggers a reply-write back to
Google (not confirmed exhaustively — grep found no `PUT`/`PATCH`/reply-related
call in `google_business.go` beyond the read paths listed above).

## 6. Proposed integration outline (outline only, no code)

Given §5, most of the plumbing (OAuth, token storage, provider interface,
DB tables) already exists. The remaining work is additive, not a rewrite:

- **New provider methods** (extend `SocialMediaProvider` interface and
  `GoogleBusinessProvider`): `ReplyToReview(accessToken, reviewName, replyText)`
  (v4 `PUT .../reviews/{review}/reply`), `CreateLocalPost(...)` (federated
  Local Posts API), `UpdateLocationInfo(...)` (Business Information API PATCH).
- **New routes** (under existing `/dashboard/*` merchant area, matching
  existing HTMX patterns): something like `POST /api/reviews/:id/reply`
  (writes back to Google via the new provider method, then updates
  `synced_reviews.review_reply` locally), `POST /api/social/google/posts`
  for creating a post.
- **New env vars**: none beyond the existing `GOOGLE_CLIENT_ID` /
  `GOOGLE_CLIENT_SECRET` / `GOOGLE_REDIRECT_URI` unless you split scopes per
  feature — reuse what's there.
- **No new tables strictly required** for reply-writing (reuse
  `synced_reviews.review_reply`); a post-creation feature would want a new
  `synced_posts` table if posts need to be tracked/displayed, but that's a
  separate scope decision, not implied by "manage reviews."
- **Sync trigger**: existing `sync_logs` table implies a scheduled/manual sync
  job already exists or is planned — new write-actions (reply, post) should
  log to the same table for consistency rather than inventing a parallel log.

This section deliberately stops at outline level per the task's scope — no
code was written.

## 7. Risks / gotchas

- **Approval rejection or multi-week delay** blocks the whole feature; start
  the access-request form (§2) in parallel with any coding, today, not after
  the code is "ready."
- **Restricted-scope OAuth verification + annual CASA assessment** is an
  ongoing compliance cost (money + process), not a one-time gate — budget for
  renewal every 12 months for as long as this feature exists.
- **Agency-reselling policy ambiguity** (§4): if Google's policy team says
  Option A is not permitted for unrelated third-party clients, the whole
  onboarding flow changes (each client needs their own approved project —
  Option B), which is a 3x-plus increase in scope. This should be resolved
  with Google directly before heavy investment, per judgment-matrix C4.
- **Shared quota ceiling** (300 QPM/project default) is a real, already-
  reported-in-the-wild scaling limit for multi-location agencies; plan sync
  frequency (e.g., hourly batched review pulls, not per-request) accordingly.
- **Review-reply policy limits**: Google enforces its own content policy on
  review replies (no spam, no incentivized-review language, etc.) and can
  reject/remove replies; this is a product-policy risk to communicate to
  merchants, not something the app can fully prevent.
- **Legacy v4 Reviews endpoint** has no published sunset date but is legacy
  infrastructure — a future deprecation notice is plausible and should be
  watched for (subscribe to Google's API deprecation announcements).
- **Q&A API is gone** (Nov 2025) — do not scope Q&A management into this
  feature; Google is replacing user-maintained Q&A with AI/Gemini-driven
  answers, not an API developers can call.
- **No API path to delete/edit a review's original text** — only reply/delete-
  reply. Don't promise merchants "remove a bad review" as a capability.
- Existing repo code stores tokens with a `TokenEncryptor` abstraction
  (`social_media/provider.go` `SyncService` takes an `encryptor
  TokenEncryptor`) — confirm this is actually wired to real encryption (not
  verified in this pass; worth a follow-up grep before shipping real tokens).

## 8. Sources (accessed 2026-07-11)

- https://developers.google.com/my-business/ref_overview — API family overview
- https://developers.google.com/my-business/content/sunset-dates — deprecation/sunset history
- https://developers.google.com/my-business/reference/rpc/google.mybusiness.v4 — legacy v4 reference (reviews)
- https://developers.google.com/my-business/content/prereqs — access prerequisites
- https://support.google.com/business/workflow/16726127 — access request form workflow
- https://legalclarity.org/how-to-complete-the-google-business-profile-api-access-request-form/ — secondary source on rejection reasons/timeline (unverified against Google primary)
- https://developers.google.com/my-business/content/implement-oauth — OAuth scope details
- https://developers.google.com/identity/protocols/oauth2/production-readiness/restricted-scope-verification — restricted scope verification process
- https://support.google.com/cloud/answer/13465431 — CASA security assessment requirement
- https://deepstrike.io/blog/google-casa-security-assessment-2025 — secondary CASA explainer
- https://developers.google.com/my-business/content/limits — quota limits (300 QPM default etc.)
- https://discuss.google.dev/t/business-profile-api-allowlisting-quota-issue-for-automated-review-replies-across-31-gbp-locations/366695 — live 2026 forum report of agency quota friction
- https://pkg.go.dev/google.golang.org/api/mybusinessaccountmanagement/v1 — official Go client (Account Management)
- https://pkg.go.dev/google.golang.org/api/businessprofileperformance/v1 — official Go client (Performance)
- https://ppc.land/google-discontinues-business-profile-q-a-api-effective-november-3/ — Q&A API discontinuation (Nov 3, 2025)
- https://developers.google.com/my-business/content/policies — Business Profile API policies (agency/reselling language)
- https://slashpost.ai/blogs/google-business-profile/google-business-profile-api-documentation-2026 — secondary source on review-reply capabilities (not independently confirmed on a Google primary page)
