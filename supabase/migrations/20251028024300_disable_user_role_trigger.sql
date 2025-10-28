-- Disable the automatic user_role creation trigger
-- We now handle role creation manually via the application

DROP TRIGGER IF EXISTS on_auth_user_created_role ON auth.users;
DROP FUNCTION IF EXISTS public.handle_new_user_role();

-- Add comment explaining why trigger was removed
COMMENT ON TABLE public.user_roles IS 'Stores user roles securely. Roles are created manually by the application, not via trigger.';
