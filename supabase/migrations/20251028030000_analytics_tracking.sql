-- Migration: Analytics Tracking Tables
-- Created: 2025-10-28
-- Description: Tables for tracking page views and link clicks for merchant analytics

-- Create page_views table
CREATE TABLE IF NOT EXISTS public.page_views (
    id BIGSERIAL PRIMARY KEY,
    merchant_id INTEGER NOT NULL REFERENCES public.merchants(id) ON DELETE CASCADE,
    ip_address INET,
    user_agent TEXT,
    referrer TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Create link_clicks table
CREATE TABLE IF NOT EXISTS public.link_clicks (
    id BIGSERIAL PRIMARY KEY,
    merchant_id INTEGER NOT NULL REFERENCES public.merchants(id) ON DELETE CASCADE,
    platform VARCHAR(50) NOT NULL, -- 'facebook', 'instagram', 'google', 'whatsapp', 'tiktok', 'xiaohongshu', etc.
    link_type VARCHAR(50) DEFAULT 'social', -- 'social', 'review', 'contact', etc.
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Create indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_page_views_merchant_id ON public.page_views(merchant_id);
CREATE INDEX IF NOT EXISTS idx_page_views_created_at ON public.page_views(created_at);
CREATE INDEX IF NOT EXISTS idx_page_views_merchant_created ON public.page_views(merchant_id, created_at);

CREATE INDEX IF NOT EXISTS idx_link_clicks_merchant_id ON public.link_clicks(merchant_id);
CREATE INDEX IF NOT EXISTS idx_link_clicks_platform ON public.link_clicks(platform);
CREATE INDEX IF NOT EXISTS idx_link_clicks_created_at ON public.link_clicks(created_at);
CREATE INDEX IF NOT EXISTS idx_link_clicks_merchant_created ON public.link_clicks(merchant_id, created_at);

-- Add comments for documentation
COMMENT ON TABLE public.page_views IS 'Tracks page views for merchant public pages';
COMMENT ON TABLE public.link_clicks IS 'Tracks clicks on social media links and review buttons';

COMMENT ON COLUMN public.page_views.merchant_id IS 'Reference to the merchant whose page was viewed';
COMMENT ON COLUMN public.page_views.ip_address IS 'IP address of the visitor (for unique visitor counting)';
COMMENT ON COLUMN public.page_views.user_agent IS 'Browser/device information';
COMMENT ON COLUMN public.page_views.referrer IS 'Where the visitor came from (HTTP referrer)';

COMMENT ON COLUMN public.link_clicks.merchant_id IS 'Reference to the merchant whose link was clicked';
COMMENT ON COLUMN public.link_clicks.platform IS 'Platform name: facebook, instagram, google, whatsapp, tiktok, xiaohongshu, threads, website, etc.';
COMMENT ON COLUMN public.link_clicks.link_type IS 'Type of link: social, review, contact';
COMMENT ON COLUMN public.link_clicks.ip_address IS 'IP address of the visitor';
COMMENT ON COLUMN public.link_clicks.user_agent IS 'Browser/device information';
