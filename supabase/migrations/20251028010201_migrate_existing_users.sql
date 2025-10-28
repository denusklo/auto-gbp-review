-- Migration: Migrate existing users to user_roles table
-- This migration moves role data from user_metadata to the secure user_roles table

-- =====================================================
-- Migrate existing auth.users to user_roles table
-- =====================================================

-- Insert roles for existing users from their metadata
INSERT INTO public.user_roles (user_id, role, created_at, updated_at)
SELECT
  id AS user_id,
  CASE
    -- Check if role exists in raw_user_meta_data
    WHEN raw_user_meta_data->>'role' = 'admin' THEN 'admin'::user_role_enum
    WHEN raw_user_meta_data->>'role' = 'superadmin' THEN 'superadmin'::user_role_enum
    WHEN raw_user_meta_data->>'role' = 'merchant' THEN 'merchant'::user_role_enum
    -- Also check raw_app_meta_data (in case some were stored there)
    WHEN raw_app_meta_data->>'role' = 'admin' THEN 'admin'::user_role_enum
    WHEN raw_app_meta_data->>'role' = 'superadmin' THEN 'superadmin'::user_role_enum
    WHEN raw_app_meta_data->>'role' = 'merchant' THEN 'merchant'::user_role_enum
    -- Default to merchant
    ELSE 'merchant'::user_role_enum
  END AS role,
  created_at,
  NOW() AS updated_at
FROM auth.users
ON CONFLICT (user_id) DO UPDATE
  SET role = EXCLUDED.role,
      updated_at = NOW();

-- =====================================================
-- Optional: Set initial superadmin
-- =====================================================

-- IMPORTANT: Update this email to your superadmin email
-- You can also set this via environment variable later
-- Uncomment and modify the line below to set your superadmin:

-- UPDATE public.user_roles
-- SET role = 'superadmin'::user_role_enum, updated_at = NOW()
-- WHERE user_id = (
--   SELECT id FROM auth.users WHERE email = 'your-superadmin-email@example.com'
-- );

-- =====================================================
-- Verification queries (for manual checking)
-- =====================================================

-- You can run these queries after migration to verify:

-- Check all user roles:
-- SELECT
--   u.email,
--   ur.role,
--   ur.banned_until,
--   ur.created_at
-- FROM auth.users u
-- LEFT JOIN public.user_roles ur ON u.id = ur.user_id
-- ORDER BY ur.role, u.email;

-- Check for users without roles (shouldn't happen):
-- SELECT u.email, u.id
-- FROM auth.users u
-- LEFT JOIN public.user_roles ur ON u.id = ur.user_id
-- WHERE ur.user_id IS NULL;
