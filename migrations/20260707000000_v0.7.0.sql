-- The single migration for the 0.7.0 release. One migration per minor release,
-- named for the release (not for any one change). ALL schema changes shipping in
-- 0.7.0 accumulate in this file.

-- Self-service email change (verify-before-swap, #128): stash the requested new
-- address in pending_email until the user confirms it via the emailed link, then
-- swap email = pending_email. A typo can't lock anyone out of their login identity.
ALTER TABLE users ADD COLUMN IF NOT EXISTS pending_email TEXT;

-- No two accounts may have the same pending change in flight — the DB rejects the
-- second claim at request time (SetPendingEmail surfaces it as ErrEmailTaken)
-- instead of letting both proceed to a confusing failure at confirmation time.
CREATE UNIQUE INDEX IF NOT EXISTS users_pending_email_unique
    ON users (pending_email) WHERE pending_email IS NOT NULL;

-- The email-change confirmation link reuses the email_verifications table with a
-- new purpose; widen the CHECK constraint to admit it (#128).
ALTER TABLE email_verifications DROP CONSTRAINT IF EXISTS email_verifications_purpose_check;
ALTER TABLE email_verifications ADD CONSTRAINT email_verifications_purpose_check
    CHECK (purpose IN ('verify', 'reset', 'email_change'));

-- Change requests can be a normal change or an access request. Same approval
-- flow; specifics (system, grantee, duration) live in the existing notes — no
-- access-request-only structured fields. A CHECK constrains the value at the DB
-- level (defense-in-depth, matching every other status/category column); add new
-- kinds here + in db.ChangeTypes to extend.
ALTER TABLE change_requests ADD COLUMN IF NOT EXISTS type TEXT NOT NULL DEFAULT 'change';
ALTER TABLE change_requests DROP CONSTRAINT IF EXISTS change_requests_type_check;
ALTER TABLE change_requests ADD CONSTRAINT change_requests_type_check
    CHECK (type IN ('change', 'access_request'));
