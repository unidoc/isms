-- The single migration for the 0.7.0 release. One migration per minor release,
-- named for the release (not for any one change). ALL schema changes shipping in
-- 0.7.0 accumulate in this file.

-- Self-service email change (verify-before-swap, #128): stash the requested new
-- address in pending_email until the user confirms it via the emailed link, then
-- swap email = pending_email. A typo can't lock anyone out of their login identity.
ALTER TABLE users ADD COLUMN IF NOT EXISTS pending_email TEXT;

-- The email-change confirmation link reuses the email_verifications table with a
-- new purpose; widen the CHECK constraint to admit it (#128).
ALTER TABLE email_verifications DROP CONSTRAINT IF EXISTS email_verifications_purpose_check;
ALTER TABLE email_verifications ADD CONSTRAINT email_verifications_purpose_check
    CHECK (purpose IN ('verify', 'reset', 'email_change'));

-- Change requests can be a normal change or an access request. Same approval
-- flow; specifics (system, grantee, duration) live in the existing notes — no
-- access-request-only structured fields.
ALTER TABLE change_requests ADD COLUMN IF NOT EXISTS type TEXT NOT NULL DEFAULT 'change';
