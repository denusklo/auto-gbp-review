-- Social Media API Integration Schema
-- This migration creates tables for storing API connections, synced reviews, and sync logs

-- API Connections Table
-- Stores OAuth credentials and connection status for each merchant's social media accounts
CREATE TABLE IF NOT EXISTS api_connections (
    id SERIAL PRIMARY KEY,
    merchant_id INTEGER NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    platform VARCHAR(50) NOT NULL CHECK (platform IN ('google_business', 'facebook', 'instagram')),
    platform_account_id VARCHAR(255), -- External account ID from the platform
    platform_account_name VARCHAR(255), -- Account display name
    access_token TEXT, -- Encrypted OAuth access token
    refresh_token TEXT, -- Encrypted OAuth refresh token
    token_expires_at TIMESTAMP WITH TIME ZONE,
    is_active BOOLEAN DEFAULT true,
    last_sync_at TIMESTAMP WITH TIME ZONE,
    sync_status VARCHAR(50) DEFAULT 'pending' CHECK (sync_status IN ('pending', 'syncing', 'completed', 'failed')),
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(merchant_id, platform, platform_account_id)
);

-- Synced Reviews Table
-- Stores reviews fetched from social media platforms
CREATE TABLE IF NOT EXISTS synced_reviews (
    id SERIAL PRIMARY KEY,
    merchant_id INTEGER NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    api_connection_id INTEGER REFERENCES api_connections(id) ON DELETE SET NULL,
    platform VARCHAR(50) NOT NULL CHECK (platform IN ('google_business', 'facebook', 'instagram')),
    platform_review_id VARCHAR(255) NOT NULL, -- Unique ID from the platform
    author_name VARCHAR(255),
    author_photo_url VARCHAR(500),
    rating DECIMAL(2,1) CHECK (rating >= 0 AND rating <= 5), -- Star rating (0-5)
    review_text TEXT,
    review_reply TEXT, -- Merchant's reply if any
    reviewed_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    is_visible BOOLEAN DEFAULT true,
    metadata JSONB DEFAULT '{}', -- Store platform-specific data
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(platform, platform_review_id)
);

-- Sync Logs Table
-- Audit trail for all sync operations
CREATE TABLE IF NOT EXISTS sync_logs (
    id SERIAL PRIMARY KEY,
    api_connection_id INTEGER NOT NULL REFERENCES api_connections(id) ON DELETE CASCADE,
    sync_type VARCHAR(50) NOT NULL CHECK (sync_type IN ('manual', 'scheduled', 'webhook')),
    status VARCHAR(50) NOT NULL CHECK (status IN ('started', 'completed', 'failed')),
    reviews_fetched INTEGER DEFAULT 0,
    reviews_added INTEGER DEFAULT 0,
    reviews_updated INTEGER DEFAULT 0,
    error_message TEXT,
    started_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE
);

-- Create indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_api_connections_merchant_id ON api_connections(merchant_id);
CREATE INDEX IF NOT EXISTS idx_api_connections_platform ON api_connections(platform);
CREATE INDEX IF NOT EXISTS idx_api_connections_is_active ON api_connections(is_active);
CREATE INDEX IF NOT EXISTS idx_api_connections_last_sync ON api_connections(last_sync_at);

CREATE INDEX IF NOT EXISTS idx_synced_reviews_merchant_id ON synced_reviews(merchant_id);
CREATE INDEX IF NOT EXISTS idx_synced_reviews_platform ON synced_reviews(platform);
CREATE INDEX IF NOT EXISTS idx_synced_reviews_is_visible ON synced_reviews(is_visible);
CREATE INDEX IF NOT EXISTS idx_synced_reviews_reviewed_at ON synced_reviews(reviewed_at DESC);
CREATE INDEX IF NOT EXISTS idx_synced_reviews_api_connection ON synced_reviews(api_connection_id);

CREATE INDEX IF NOT EXISTS idx_sync_logs_api_connection ON sync_logs(api_connection_id);
CREATE INDEX IF NOT EXISTS idx_sync_logs_status ON sync_logs(status);
CREATE INDEX IF NOT EXISTS idx_sync_logs_started_at ON sync_logs(started_at DESC);

-- Create trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_api_connections_updated_at BEFORE UPDATE ON api_connections
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_synced_reviews_updated_at BEFORE UPDATE ON synced_reviews
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Add RLS (Row Level Security) policies
ALTER TABLE api_connections ENABLE ROW LEVEL SECURITY;
ALTER TABLE synced_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE sync_logs ENABLE ROW LEVEL SECURITY;

-- Policy: Merchants can only see their own API connections
CREATE POLICY api_connections_merchant_policy ON api_connections
    FOR ALL
    USING (
        merchant_id IN (
            SELECT id FROM merchants WHERE auth_user_id = auth.uid()
        )
    );

-- Policy: Admins can see all API connections
CREATE POLICY api_connections_admin_policy ON api_connections
    FOR ALL
    USING (
        EXISTS (
            SELECT 1 FROM auth.users
            WHERE id = auth.uid()
            AND raw_user_meta_data->>'role' = 'admin'
        )
    );

-- Policy: Merchants can only see their own synced reviews
CREATE POLICY synced_reviews_merchant_policy ON synced_reviews
    FOR ALL
    USING (
        merchant_id IN (
            SELECT id FROM merchants WHERE auth_user_id = auth.uid()
        )
    );

-- Policy: Admins can see all synced reviews
CREATE POLICY synced_reviews_admin_policy ON synced_reviews
    FOR ALL
    USING (
        EXISTS (
            SELECT 1 FROM auth.users
            WHERE id = auth.uid()
            AND raw_user_meta_data->>'role' = 'admin'
        )
    );

-- Policy: View sync logs for own API connections
CREATE POLICY sync_logs_merchant_policy ON sync_logs
    FOR SELECT
    USING (
        api_connection_id IN (
            SELECT ac.id FROM api_connections ac
            INNER JOIN merchants m ON ac.merchant_id = m.id
            WHERE m.auth_user_id = auth.uid()
        )
    );

-- Policy: Admins can see all sync logs
CREATE POLICY sync_logs_admin_policy ON sync_logs
    FOR ALL
    USING (
        EXISTS (
            SELECT 1 FROM auth.users
            WHERE id = auth.uid()
            AND raw_user_meta_data->>'role' = 'admin'
        )
    );

-- Grant permissions
GRANT ALL ON api_connections TO authenticated;
GRANT ALL ON synced_reviews TO authenticated;
GRANT SELECT ON sync_logs TO authenticated;
GRANT ALL ON sync_logs TO service_role;
