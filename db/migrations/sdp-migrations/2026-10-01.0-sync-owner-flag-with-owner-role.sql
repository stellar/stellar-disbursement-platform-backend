-- Make the owner flag match the owner role. Flagged users whose roles exclude owner were demoted
-- from the dashboard while the flag kept them owners: apply the demotion unless no owner would remain.

-- +migrate Up

-- +migrate StatementBegin
DO $$
BEGIN
    -- auth_users is missing on fresh tenant schemas (sdp migrations run before auth migrations there).
    IF to_regclass('auth_users') IS NULL THEN
        RETURN;
    END IF;

    -- Demote only if an active owner remains: one holding the owner role, or flagged with no roles.
    IF EXISTS (
        SELECT 1 FROM auth_users
        WHERE is_active
          AND ('owner' = ANY(COALESCE(roles, '{}')) OR (is_owner AND cardinality(COALESCE(roles, '{}')) = 0))
    ) THEN
        INSERT INTO wallet_memberships (user_id, wallet_id, role)
        SELECT u.id, w.id, r.role
        FROM auth_users u
        CROSS JOIN LATERAL unnest(u.roles) AS r(role)
        JOIN distribution_wallets w ON w.is_default
        WHERE u.is_owner
          AND NOT 'owner' = ANY(u.roles)
          AND r.role IN ('financial_controller', 'developer', 'business', 'initiator', 'approver')
        ON CONFLICT (user_id, wallet_id, role) DO NOTHING;

        UPDATE auth_users
        SET is_owner = false
        WHERE is_owner
          AND cardinality(COALESCE(roles, '{}')) > 0
          AND NOT 'owner' = ANY(roles);
    END IF;

    -- Everyone still an owner (by flag or role) gets both, and only the owner role.
    UPDATE auth_users
    SET
        is_owner = true,
        roles = ARRAY['owner']
    WHERE is_owner OR 'owner' = ANY(COALESCE(roles, '{}'));
END
$$;
-- +migrate StatementEnd

-- +migrate Down
-- No down migration needed as the previous flag/role mismatch cannot be restored.
