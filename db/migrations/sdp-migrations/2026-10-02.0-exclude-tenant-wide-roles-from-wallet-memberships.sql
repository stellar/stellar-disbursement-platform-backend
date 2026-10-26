-- Owner and developer are tenant-wide: they hold no memberships, and the developer membership role is retired.
-- Non-developers lose their developer grants and must be reassigned a role from the dashboard.

-- +migrate Up

-- +migrate StatementBegin
DO $$
DECLARE n_removed int;
BEGIN
    IF to_regclass('auth_users') IS NOT NULL THEN
        DELETE FROM wallet_memberships m
        USING auth_users u
        WHERE m.user_id = u.id
          AND (u.is_owner OR 'owner' = ANY(u.roles) OR 'developer' = ANY(u.roles));
    END IF;

    DELETE FROM wallet_memberships WHERE role = 'developer';
    GET DIAGNOSTICS n_removed = ROW_COUNT;
    IF n_removed > 0 THEN
        RAISE WARNING '%: % developer membership(s) held by non-developers were removed; re-grant access from the dashboard', current_schema(), n_removed;
    END IF;
END
$$;
-- +migrate StatementEnd

ALTER TABLE wallet_memberships DROP CONSTRAINT wallet_memberships_role_check;
ALTER TABLE wallet_memberships ADD CONSTRAINT wallet_memberships_role_check
    CHECK (role IN ('financial_controller', 'business', 'initiator', 'approver'));

-- +migrate Down

ALTER TABLE wallet_memberships DROP CONSTRAINT wallet_memberships_role_check;
ALTER TABLE wallet_memberships ADD CONSTRAINT wallet_memberships_role_check
    CHECK (role IN ('financial_controller', 'developer', 'business', 'initiator', 'approver'));
