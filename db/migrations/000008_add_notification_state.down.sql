DROP TABLE IF EXISTS notification_log;

DROP INDEX IF EXISTS idx_purchase_unfulfilled_paid;
DROP INDEX IF EXISTS idx_purchase_abandoned_candidates;

ALTER TABLE purchase
    DROP COLUMN IF EXISTS abandoned_claimed_at,
    DROP COLUMN IF EXISTS abandoned_notified_at,
    DROP COLUMN IF EXISTS failed_claimed_at,
    DROP COLUMN IF EXISTS failed_notified_at,
    DROP COLUMN IF EXISTS fulfilled_at;
