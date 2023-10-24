-- +migrate Up
ALTER TABLE
  public.organizations
ADD
  COLUMN payment_cancellation_period int;

-- +migrate Down
ALTER TABLE
  public.organizations DROP COLUMN payment_cancellation_period;