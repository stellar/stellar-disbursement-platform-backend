-- +migrate Up

CREATE INDEX payments_stellar_transaction_id_idx ON payments (stellar_transaction_id);

-- +migrate Down

DROP INDEX IF EXISTS payments_stellar_transaction_id_idx;
