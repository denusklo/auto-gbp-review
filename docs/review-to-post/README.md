# Review-to-Post: Turn Synced Reviews into Facebook Marketing Posts

Status: **PROPOSED** — requirements captured 2026-07-12, not yet implemented.
Origin: user discussion after Facebook posting went live (2026-07-11/12).

## Problem

Merchants collect reviews on Google / Facebook, but those reviews just sit on the
platform. The product's promise is "turn reviews into marketing assets" — nothing
does that today.

An earlier idea — auto-post every incoming review — was **explicitly rejected by
the user**: 100 good reviews would mean 100 posts on the merchant's page. Curation
is a hard requirement.

## What already exists (all verified working 2026-07-12)

- **Review sync (in):** `social_media/facebook.go` `FetchReviews` +
  `social_media/provider.go` SyncService → `synced_reviews` table.
  Requires the `pages_read_user_content` scope (added to OAuth 2026-07-12).
- **Posting (out):** `FacebookProvider.PublishPost` (text + photo) →
  `POST /api/social-media/connections/:id/posts`, history in `published_posts`.
- **Merchant dashboard** with integrations page at `/dashboard/integrations`.

The feature is essentially "one button between two working pipelines."

## Requirements

1. **Curated, never automatic-per-review.** Merchant explicitly picks which
   reviews become posts (v1). Optional later: weekly auto-digest of top N
   reviews, opt-in per merchant.
2. **Share button per synced review** in the dashboard (integrations page or a
   reviews list): "Share to Facebook".
3. **Post composition:** template like
   `⭐⭐⭐⭐⭐ "{review_text}" — {author_name}` + a merchant-editable thank-you
   line. Merchant can edit before publishing (post preview).
4. **Idempotence:** a review already shared shows "Shared" instead of the button
   (track `synced_review_id` → `published_posts` link, e.g. nullable
   `source_review_id` column on `published_posts`).
5. **Only good reviews surfaced by default** (rating ≥ 4) but merchant can share
   any.
6. Platform: Facebook first (works today). Instagram/Threads later once their
   posting providers exist (see docs/meta-integration/README.md §6).

## Proposed v1 slice (ponytail-sized)

- Add `source_review_id INT NULL REFERENCES synced_reviews(id)` to
  `published_posts` (one migration).
- Integrations page: recent synced reviews fragment gets a "Share to Facebook"
  button per review (htmx POST).
- New handler: builds the post text from the review, calls the existing
  `PublishPost` path, records `source_review_id`.
- Skip for v1: editing/preview UI, digests, image cards, Instagram.

## Open decisions (user input needed before building)

- Post text template wording (merchant-facing copy — taste decision).
- Where the share button lives: integrations page vs. a dedicated reviews page.
- Whether Google-synced reviews may be posted to Facebook (cross-platform
  quoting) — policy-wise fine (it's the merchant quoting their own customer),
  but user should confirm intent.

## Related notes

- Deep-link improvement shipped 2026-07-12: public "Write a Review" button for
  Facebook now prefers the OAuth-connected page's reviews tab
  (`https://www.facebook.com/profile.php?id=<PAGE_ID>&sk=reviews`) over the
  manually entered `facebook_url` (handlers.go `GetReviewModal`).
- Review submission CANNOT be automated on the platforms (no write APIs for
  reviews on Google or Facebook; anti-fraud by design). The link-out flow is the
  only compliant approach. Documented so nobody re-researches this.
