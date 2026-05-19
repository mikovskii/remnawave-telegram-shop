BEGIN;

DROP INDEX IF EXISTS idx_referral_paid_at;

ALTER TABLE referral
    DROP COLUMN IF EXISTS earned_days,
    DROP COLUMN IF EXISTS paid_at;

COMMIT;
