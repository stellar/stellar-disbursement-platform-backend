-- Reserves an events row for the duration of a delivery attempt rather than for the microseconds
-- the claim statement holds its row locks. Every SDP replica runs the scheduler (see cmd/serve.go),
-- so without a persisted lease each replica's tick re-claims the same undelivered rows: the target
-- receives duplicate concurrent POSTs and each row burns one attempt per replica per interval,
-- shrinking the retry budget in proportion to replica count exactly when a target is failing.
-- The lease self-expires — a claim counts only while claimed_until is in the future — so a replica
-- that crashes mid-batch strands nothing and no reaper is needed to clean up after it.

-- +migrate Up

ALTER TABLE events ADD COLUMN claimed_until TIMESTAMPTZ;

-- +migrate Down

ALTER TABLE events DROP COLUMN claimed_until;
