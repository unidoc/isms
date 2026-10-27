-- 0.9.0, second file: the weekly personal digest (#232). A separate file from
-- 20261026120000_v0.9.0.sql so it cannot conflict with other changes to that one.
--
-- digest_enabled is the member's per-organization opt-out. Default on: a
-- digest is only sent when something is waiting on the member, and every mail
-- links to Settings, where it can be turned off.
ALTER TABLE organization_members
    ADD COLUMN IF NOT EXISTS digest_enabled BOOLEAN NOT NULL DEFAULT true;

-- One row per (organization, member, ISO week) records that the week's digest
-- was handled, so two runs of `isms server manager`, or two replicas, cannot
-- send the same week twice. The row is claimed with INSERT ... ON CONFLICT DO
-- NOTHING before the mail goes out. outcome 'sent' is a mail that went out;
-- 'empty' records that the member had nothing pending, so an item arriving
-- later the same week does not trigger a mid-week digest.
CREATE TABLE IF NOT EXISTS digest_sends (
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    period          TEXT NOT NULL,
    outcome         TEXT NOT NULL CHECK (outcome IN ('sent', 'empty')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id, period)
);

-- Same tenant_isolation policy as every org-scoped table. CREATE POLICY has no
-- IF NOT EXISTS, so it is guarded for re-runs.
ALTER TABLE digest_sends ENABLE ROW LEVEL SECURITY;
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies
                    WHERE tablename = 'digest_sends' AND policyname = 'tenant_isolation') THEN
        CREATE POLICY tenant_isolation ON digest_sends
            USING (organization_id = current_setting('app.current_org_id', true)::INTEGER)
            WITH CHECK (organization_id = current_setting('app.current_org_id', true)::INTEGER);
    END IF;
END $$;
