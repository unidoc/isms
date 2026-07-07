-- DB-level backstop for change_requests.type (defense-in-depth, matching every
-- other status/category column). This is a SEPARATE migration, not an amendment
-- to 20260707000000_v0.7.0.sql, because that file already shipped on master (#133)
-- and the runner skips already-applied files by name — so amending it would never
-- reach databases that already ran v0.7.0. This new file runs everywhere.
--
-- DROP IF EXISTS makes it safe on fresh DBs too; existing rows all satisfy it
-- (type is NOT NULL DEFAULT 'change' and every write path is enum-validated).
-- Extend by adding a value here + in db.ChangeTypes.
ALTER TABLE change_requests DROP CONSTRAINT IF EXISTS change_requests_type_check;
ALTER TABLE change_requests ADD CONSTRAINT change_requests_type_check
    CHECK (type IN ('change', 'access_request'));
