-- Migration: Create user_roles and audit_logs tables for secure role management
-- This migration moves role management from user_metadata to a dedicated table
-- to prevent users from modifying their own roles

-- =====================================================
-- 1. Create user_roles table for secure role management
-- =====================================================

CREATE TYPE user_role_enum AS ENUM ('superadmin', 'admin', 'merchant');

CREATE TABLE IF NOT EXISTS public.user_roles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  role user_role_enum NOT NULL DEFAULT 'merchant',
  banned_until TIMESTAMPTZ,
  banned_reason TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT unique_user_role UNIQUE(user_id)
);

-- Add indexes for performance
CREATE INDEX IF NOT EXISTS idx_user_roles_user_id ON public.user_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_role ON public.user_roles(role);
CREATE INDEX IF NOT EXISTS idx_user_roles_banned ON public.user_roles(banned_until) WHERE banned_until IS NOT NULL;

-- Add RLS policies (important for security)
ALTER TABLE public.user_roles ENABLE ROW LEVEL SECURITY;

-- Allow users to view their own role only
CREATE POLICY "Users can view their own role"
  ON public.user_roles
  FOR SELECT
  TO authenticated
  USING (auth.uid() = user_id);

-- Only allow supabase_auth_admin to modify (via Auth Hook)
GRANT SELECT ON public.user_roles TO authenticated;
GRANT ALL ON public.user_roles TO supabase_auth_admin;

-- Revoke public access
REVOKE ALL ON public.user_roles FROM anon, public;

-- =====================================================
-- 2. Create audit_logs table for tracking admin actions
-- =====================================================

CREATE TABLE IF NOT EXISTS public.audit_logs (
  id BIGSERIAL PRIMARY KEY,
  user_id UUID REFERENCES auth.users(id) ON DELETE SET NULL,
  user_email TEXT,
  action TEXT NOT NULL,
  target_type TEXT,
  target_id TEXT,
  details JSONB,
  ip_address INET,
  user_agent TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Add indexes for performance
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON public.audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON public.audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON public.audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_target ON public.audit_logs(target_type, target_id);

-- Add RLS policies
ALTER TABLE public.audit_logs ENABLE ROW LEVEL SECURITY;

-- Only admins and superadmins can view audit logs
CREATE POLICY "Admins can view audit logs"
  ON public.audit_logs
  FOR SELECT
  TO authenticated
  USING (
    EXISTS (
      SELECT 1 FROM public.user_roles
      WHERE user_id = auth.uid()
      AND role IN ('admin', 'superadmin')
    )
  );

-- Only allow service role to insert (via application code)
REVOKE ALL ON public.audit_logs FROM authenticated, anon, public;
GRANT SELECT ON public.audit_logs TO authenticated;

-- =====================================================
-- 3. Helper function to check if user is banned
-- =====================================================

CREATE OR REPLACE FUNCTION public.is_user_banned(check_user_id UUID)
RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
STABLE
AS $$
DECLARE
  ban_time TIMESTAMPTZ;
BEGIN
  SELECT banned_until INTO ban_time
  FROM public.user_roles
  WHERE user_id = check_user_id;

  RETURN (ban_time IS NOT NULL AND ban_time > NOW());
END;
$$;

-- Grant execute permission
GRANT EXECUTE ON FUNCTION public.is_user_banned TO authenticated, supabase_auth_admin;

-- =====================================================
-- 4. Helper function to get user role
-- =====================================================

CREATE OR REPLACE FUNCTION public.get_user_role_secure(check_user_id UUID)
RETURNS TEXT
LANGUAGE plpgsql
SECURITY DEFINER
STABLE
AS $$
DECLARE
  user_role_value TEXT;
BEGIN
  SELECT role::TEXT INTO user_role_value
  FROM public.user_roles
  WHERE user_id = check_user_id;

  RETURN COALESCE(user_role_value, 'merchant');
END;
$$;

-- Grant execute permission
GRANT EXECUTE ON FUNCTION public.get_user_role_secure TO authenticated, supabase_auth_admin;

-- =====================================================
-- 5. Function to log audit events
-- =====================================================

CREATE OR REPLACE FUNCTION public.log_audit_event(
  p_action TEXT,
  p_target_type TEXT DEFAULT NULL,
  p_target_id TEXT DEFAULT NULL,
  p_details JSONB DEFAULT NULL,
  p_ip_address INET DEFAULT NULL,
  p_user_agent TEXT DEFAULT NULL
)
RETURNS BIGINT
LANGUAGE plpgsql
SECURITY DEFINER
AS $$
DECLARE
  log_id BIGINT;
  current_user_email TEXT;
BEGIN
  -- Get current user's email
  SELECT email INTO current_user_email
  FROM auth.users
  WHERE id = auth.uid();

  -- Insert audit log
  INSERT INTO public.audit_logs (
    user_id,
    user_email,
    action,
    target_type,
    target_id,
    details,
    ip_address,
    user_agent
  )
  VALUES (
    auth.uid(),
    current_user_email,
    p_action,
    p_target_type,
    p_target_id,
    p_details,
    p_ip_address,
    p_user_agent
  )
  RETURNING id INTO log_id;

  RETURN log_id;
END;
$$;

-- Grant execute to authenticated users
GRANT EXECUTE ON FUNCTION public.log_audit_event TO authenticated;

-- =====================================================
-- 6. Trigger to automatically update updated_at timestamp
-- =====================================================

CREATE OR REPLACE FUNCTION public.update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_user_roles_updated_at
  BEFORE UPDATE ON public.user_roles
  FOR EACH ROW
  EXECUTE FUNCTION public.update_updated_at_column();

-- =====================================================
-- 7. Trigger to create user_role entry for new auth users
-- =====================================================

CREATE OR REPLACE FUNCTION public.handle_new_user_role()
RETURNS TRIGGER AS $$
DECLARE
  user_role_value user_role_enum;
BEGIN
  -- Get role from user metadata (for backward compatibility during migration)
  -- Default to 'merchant' if not specified
  user_role_value := COALESCE(
    (NEW.raw_user_meta_data->>'role')::user_role_enum,
    'merchant'::user_role_enum
  );

  -- Insert into user_roles table
  INSERT INTO public.user_roles (user_id, role)
  VALUES (NEW.id, user_role_value)
  ON CONFLICT (user_id) DO NOTHING;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Create the trigger
DROP TRIGGER IF EXISTS on_auth_user_created_role ON auth.users;
CREATE TRIGGER on_auth_user_created_role
  AFTER INSERT ON auth.users
  FOR EACH ROW
  EXECUTE FUNCTION public.handle_new_user_role();

-- Grant necessary permissions for the trigger
GRANT USAGE ON SCHEMA public TO supabase_auth_admin;
GRANT ALL ON public.user_roles TO supabase_auth_admin;

-- =====================================================
-- 8. Comments for documentation
-- =====================================================

COMMENT ON TABLE public.user_roles IS 'Stores user roles securely. Users cannot modify their own roles.';
COMMENT ON TABLE public.audit_logs IS 'Tracks all administrative actions for security and compliance.';
COMMENT ON COLUMN public.user_roles.banned_until IS 'If set, user is banned until this timestamp. NULL means not banned. Use far future date for permanent ban.';
COMMENT ON FUNCTION public.is_user_banned IS 'Check if a user is currently banned.';
COMMENT ON FUNCTION public.get_user_role_secure IS 'Get user role securely from user_roles table.';
COMMENT ON FUNCTION public.log_audit_event IS 'Log an audit event. Call this from application code for important actions.';
