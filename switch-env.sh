#!/bin/bash

# Switch environment script for auto-gbp-review
# Usage: ./switch-env.sh [local|production]

ENV=$1

if [ "$ENV" = "local" ]; then
    echo "Switching to LOCAL Supabase..."
    cp .env.local .env
    echo "✅ Environment switched to LOCAL"
    echo "   - Supabase URL: http://127.0.0.1:54321"
    echo "   - Database: postgresql://postgres:postgres@127.0.0.1:54322/postgres"
elif [ "$ENV" = "production" ]; then
    echo "Switching to PRODUCTION Supabase..."
    cp .env.production .env
    echo "✅ Environment switched to PRODUCTION"
    echo "   - Supabase URL: https://lusoihisangqfpiqrctl.supabase.co"
else
    echo "❌ Invalid environment. Use 'local' or 'production'"
    echo "   Usage: ./switch-env.sh [local|production]"
    exit 1
fi

echo ""
echo "⚠️  Don't forget to restart your app for changes to take effect!"