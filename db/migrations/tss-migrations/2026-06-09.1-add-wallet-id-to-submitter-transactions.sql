-- Per-wallet observability: TSS transactions carry the source distribution wallet so
-- wallet_id tagging survives API → TSS → channel-account layers (logs + payment-flow metric
-- labels per the accepted cardinality classification). Nullable: legacy rows and
-- non-payment transaction types (wallet creation, sponsored) have no distribution wallet.

-- +migrate Up

ALTER TABLE submitter_transactions ADD COLUMN wallet_id VARCHAR(36);

-- +migrate Down

ALTER TABLE submitter_transactions DROP COLUMN wallet_id;
