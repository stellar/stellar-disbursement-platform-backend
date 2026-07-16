-- +migrate Up
-- Add APPROVED status to disbursement_status enum
-- This allows the three-step workflow: Uploader (READY) -> Approver (APPROVED) -> FinanceOfficer (STARTED)
ALTER TYPE disbursement_status ADD VALUE IF NOT EXISTS 'APPROVED' AFTER 'READY';

-- +migrate Down
-- Remove APPROVED status from disbursement_status enum
-- Note: PostgreSQL doesn't support removing enum values directly, so this is a no-op
-- ALTER TYPE disbursement_status DROP VALUE IF EXISTS 'APPROVED';