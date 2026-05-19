BEGIN;

ALTER TABLE referral
    ADD COLUMN IF NOT EXISTS paid_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS earned_days INTEGER NOT NULL DEFAULT 0;

UPDATE referral
SET paid_at = used_at
WHERE bonus_granted = TRUE
  AND paid_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_referral_paid_at ON referral (paid_at);

COMMIT;
