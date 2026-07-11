-- Published Posts Table
-- Records posts published to social media platforms (e.g. Facebook Page feed/photos)
-- Note: merchant_id / api_connection_id are INTEGER to match api_connections (SERIAL PK),
-- not UUID, matching the existing 20251028163000_social_media_integration.sql schema.
CREATE TABLE IF NOT EXISTS published_posts (
    id SERIAL PRIMARY KEY,
    merchant_id INTEGER NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    api_connection_id INTEGER REFERENCES api_connections(id) ON DELETE SET NULL,
    platform VARCHAR(50) NOT NULL,
    platform_post_id VARCHAR(255),
    content TEXT NOT NULL,
    photo_url VARCHAR(500),
    status VARCHAR(50) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'published', 'failed')),
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_published_posts_merchant_id ON published_posts(merchant_id);
CREATE INDEX IF NOT EXISTS idx_published_posts_api_connection ON published_posts(api_connection_id);

ALTER TABLE published_posts ENABLE ROW LEVEL SECURITY;

CREATE POLICY published_posts_merchant_policy ON published_posts
    FOR ALL
    USING (
        merchant_id IN (
            SELECT id FROM merchants WHERE auth_user_id = auth.uid()
        )
    );

CREATE POLICY published_posts_admin_policy ON published_posts
    FOR ALL
    USING (
        EXISTS (
            SELECT 1 FROM auth.users
            WHERE id = auth.uid()
            AND raw_user_meta_data->>'role' = 'admin'
        )
    );

GRANT ALL ON published_posts TO authenticated;
