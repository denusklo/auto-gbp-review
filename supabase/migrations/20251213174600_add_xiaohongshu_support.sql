-- Add Xiaohongshu (XHS) support to social media integration
-- This migration adds support for the xiaohongshu platform to existing constraints

-- Drop existing constraints to add xiaohongshu support
ALTER TABLE api_connections DROP CONSTRAINT IF EXISTS api_connections_platform_check;
ALTER TABLE synced_reviews DROP CONSTRAINT IF EXISTS synced_reviews_platform_check;

-- Add updated constraints that include xiaohongshu
ALTER TABLE api_connections
ADD CONSTRAINT api_connections_platform_check
CHECK (platform IN ('google_business', 'facebook', 'instagram', 'xiaohongshu'));

ALTER TABLE synced_reviews
ADD CONSTRAINT synced_reviews_platform_check
CHECK (platform IN ('google_business', 'facebook', 'instagram', 'xiaohongshu'));

-- Note: No data migration needed as this just adds support for a new platform