-- Storage bucket for merchant logo uploads (public read; app uploads via service key).
-- ON CONFLICT: no-op where the bucket already exists (e.g. prod, created via dashboard).
INSERT INTO storage.buckets (id, name, public)
VALUES ('merchant-logos', 'merchant-logos', true)
ON CONFLICT (id) DO NOTHING;
