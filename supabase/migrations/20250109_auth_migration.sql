-- Add auth_user_id column to link custom users table with Supabase Auth
ALTER TABLE users 
ADD COLUMN IF NOT EXISTS auth_user_id UUID UNIQUE REFERENCES auth.users(id) ON DELETE CASCADE;

-- Add index for faster lookups
CREATE INDEX IF NOT EXISTS idx_users_auth_user_id ON users(auth_user_id);

-- Update merchants table to support auth_user_id directly (for new users)
ALTER TABLE merchants 
ADD COLUMN IF NOT EXISTS auth_user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE;

-- Add index for merchant auth lookups
CREATE INDEX IF NOT EXISTS idx_merchants_auth_user_id ON merchants(auth_user_id);

-- Create a function to get user role from metadata
CREATE OR REPLACE FUNCTION get_user_role(user_id UUID)
RETURNS TEXT AS $$
DECLARE
    user_role TEXT;
BEGIN
    -- First check if user exists in custom users table
    SELECT role INTO user_role
    FROM users
    WHERE auth_user_id = user_id;
    
    IF user_role IS NOT NULL THEN
        RETURN user_role;
    END IF;
    
    -- Fallback to auth.users metadata
    SELECT raw_user_meta_data->>'role' INTO user_role
    FROM auth.users
    WHERE id = user_id;
    
    RETURN COALESCE(user_role, 'merchant');
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Create a trigger to automatically create a user record when someone signs up
CREATE OR REPLACE FUNCTION handle_new_user()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO users (email, auth_user_id, role, password_hash)
    VALUES (
        NEW.email,
        NEW.id,
        COALESCE(NEW.raw_user_meta_data->>'role', 'merchant'),
        'SUPABASE_AUTH' -- Placeholder to indicate Supabase Auth is handling passwords
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Create the trigger
DROP TRIGGER IF EXISTS on_auth_user_created ON auth.users;
CREATE TRIGGER on_auth_user_created
    AFTER INSERT ON auth.users
    FOR EACH ROW
    EXECUTE FUNCTION handle_new_user();

-- Migration helper: Function to migrate existing users to Supabase Auth
-- This should be run manually after setting up the Supabase Auth integration
CREATE OR REPLACE FUNCTION migrate_users_to_auth()
RETURNS TABLE(email TEXT, temp_password TEXT) AS $$
DECLARE
    user_record RECORD;
    temp_pass TEXT;
BEGIN
    FOR user_record IN SELECT id, email FROM users WHERE auth_user_id IS NULL
    LOOP
        -- Generate a temporary password for migration
        temp_pass := encode(gen_random_bytes(12), 'base64');
        
        -- Return the email and temporary password for manual processing
        email := user_record.email;
        temp_password := temp_pass;
        RETURN NEXT;
    END LOOP;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Add comment explaining the migration process
COMMENT ON FUNCTION migrate_users_to_auth() IS 
'Helper function to list existing users that need to be migrated to Supabase Auth. 
Returns email and a generated temporary password that should be used to create 
accounts in Supabase Auth, then users should be notified to reset their passwords.';