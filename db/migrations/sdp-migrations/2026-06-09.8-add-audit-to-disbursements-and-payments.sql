-- Per-wallet observability: the audit log gains the wallet dimension. Disbursements and
-- payments now have immutable audit history; both tables carry source_wallet_id, so every
-- audit row is wallet-attributable for compliance, dispute, and treasury queries.

-- +migrate Up

SELECT create_audit_table('disbursements');
SELECT create_audit_table('payments');

-- +migrate Down

SELECT drop_audit_table('payments');
SELECT drop_audit_table('disbursements');
