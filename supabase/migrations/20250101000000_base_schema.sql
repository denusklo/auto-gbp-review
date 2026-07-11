-- Base schema, extracted verbatim from database.go migrate() (legacy Go-app bootstrap).
-- Purpose: make the migration chain self-contained so shadow databases (supabase db diff)
-- and fresh local stacks can replay from zero without running the Go app first.
-- Every statement is IF NOT EXISTS, so this is a no-op on any database (including prod)
-- where the Go app already created these objects.

CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(50) DEFAULT 'merchant',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS merchants (
    id SERIAL PRIMARY KEY,
    auth_user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    business_name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE NOT NULL,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS merchant_details (
    id SERIAL PRIMARY KEY,
    merchant_id INTEGER REFERENCES merchants(id) ON DELETE CASCADE,
    address TEXT,
    phone_number VARCHAR(50),
    whatsapp_preset_text TEXT DEFAULT 'I''m interested in your services',
    facebook_url VARCHAR(500),
    xiaohongshu_id VARCHAR(255),
    tiktok_url VARCHAR(500),
    instagram_url VARCHAR(500),
    threads_url VARCHAR(500),
    website_url VARCHAR(500),
    google_play_url VARCHAR(500),
    app_store_url VARCHAR(500),
    google_maps_url VARCHAR(500),
    waze_url VARCHAR(500),
    logo_url VARCHAR(500),
    theme_color VARCHAR(7) DEFAULT '#3B82F6',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_merchants_slug ON merchants(slug);
CREATE INDEX IF NOT EXISTS idx_merchants_auth_user_id ON merchants(auth_user_id);
CREATE INDEX IF NOT EXISTS idx_merchant_details_merchant_id ON merchant_details(merchant_id);
