-- Migration: Create Custom Access Token Auth Hook
-- This hook runs before issuing JWTs and injects the user's role
-- from the user_roles table into the JWT claims

-- =====================================================
-- Custom Access Token Hook Function
-- =====================================================

CREATE OR REPLACE FUNCTION public.custom_access_token_hook(event jsonb)
RETURNS jsonb
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  claims jsonb;
  user_role_value TEXT;
  is_banned BOOLEAN;
  ban_until_value TIMESTAMPTZ;
BEGIN
  -- Get existing claims from the event
  claims := event->'claims';

  -- Fetch user role and ban status from user_roles table
  SELECT
    role::TEXT,
    (banned_until IS NOT NULL AND banned_until > NOW()),
    banned_until
  INTO
    user_role_value,
    is_banned,
    ban_until_value
  FROM public.user_roles
  WHERE user_id = (event->>'user_id')::uuid;

  -- If user is banned, return error
  IF is_banned THEN
    -- Create error response
    RETURN jsonb_build_object(
      'error', jsonb_build_object(
        'http_code', 403,
        'message', 'Your account has been suspended. Please contact support for assistance.'
      )
    );
  END IF;

  -- Set the user_role claim
  IF user_role_value IS NOT NULL THEN
    claims := jsonb_set(claims, '{user_role}', to_jsonb(user_role_value));
  ELSE
    -- Default to merchant if no role found
    claims := jsonb_set(claims, '{user_role}', '"merchant"');
  END IF;

  -- Update the claims in the event
  event := jsonb_set(event, '{claims}', claims);

  -- Return the modified event
  RETURN event;
END;
$$;

-- =====================================================
-- Grant Permissions
-- =====================================================

-- Grant necessary permissions to supabase_auth_admin
GRANT USAGE ON SCHEMA public TO supabase_auth_admin;

GRANT EXECUTE ON FUNCTION public.custom_access_token_hook TO supabase_auth_admin;

-- Grant access to user_roles table for the hook
GRANT SELECT ON public.user_roles TO supabase_auth_admin;

-- Revoke access from regular users to prevent security issues
REVOKE EXECUTE ON FUNCTION public.custom_access_token_hook FROM authenticated, anon, public;

-- =====================================================
-- Create policy to allow auth admin to read user roles
-- =====================================================

CREATE POLICY "Allow auth admin to read user roles"
  ON public.user_roles
  AS PERMISSIVE
  FOR SELECT
  TO supabase_auth_admin
  USING (true);

-- =====================================================
-- Documentation
-- =====================================================

COMMENT ON FUNCTION public.custom_access_token_hook IS
'Custom Access Token Hook that runs before issuing JWTs.
Injects user_role from user_roles table into JWT claims.
Also checks if user is banned and rejects authentication if so.
This hook must be enabled in Supabase Dashboard: Authentication > Hooks (Beta)';
