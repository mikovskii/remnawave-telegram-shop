DROP VIEW IF EXISTS analytics_monthly_revenue;
DROP VIEW IF EXISTS analytics_customer_summary;
DROP VIEW IF EXISTS analytics_daily_funnel;

DROP TABLE IF EXISTS subscription_period;
DROP TABLE IF EXISTS bot_event;

DROP INDEX IF EXISTS idx_customer_source;
DROP INDEX IF EXISTS idx_customer_campaign;
DROP INDEX IF EXISTS idx_customer_lifecycle_stage;
DROP INDEX IF EXISTS idx_customer_first_paid_at;

ALTER TABLE customer
    DROP COLUMN IF EXISTS first_seen_at,
    DROP COLUMN IF EXISTS last_seen_at,
    DROP COLUMN IF EXISTS first_paid_at,
    DROP COLUMN IF EXISTS first_start_payload,
    DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS medium,
    DROP COLUMN IF EXISTS campaign,
    DROP COLUMN IF EXISTS referrer_telegram_id,
    DROP COLUMN IF EXISTS lifecycle_stage,
    DROP COLUMN IF EXISTS lead_score;
