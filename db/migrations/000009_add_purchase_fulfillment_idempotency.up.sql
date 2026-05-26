DELETE FROM subscription_period duplicate
USING subscription_period original
WHERE duplicate.purchase_id = original.purchase_id
  AND duplicate.source_type = 'paid'
  AND original.source_type = 'paid'
  AND duplicate.purchase_id IS NOT NULL
  AND duplicate.id > original.id;

CREATE UNIQUE INDEX idx_subscription_period_paid_purchase_unique
    ON subscription_period (purchase_id)
    WHERE source_type = 'paid' AND purchase_id IS NOT NULL;
