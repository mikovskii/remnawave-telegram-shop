ALTER TABLE customer
    ADD COLUMN first_seen_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN last_seen_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN first_paid_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN first_start_payload TEXT,
    ADD COLUMN source VARCHAR(120),
    ADD COLUMN medium VARCHAR(120),
    ADD COLUMN campaign VARCHAR(120),
    ADD COLUMN referrer_telegram_id BIGINT,
    ADD COLUMN lifecycle_stage VARCHAR(40) NOT NULL DEFAULT 'new',
    ADD COLUMN lead_score INTEGER NOT NULL DEFAULT 0;

UPDATE customer
SET first_seen_at = COALESCE(first_seen_at, created_at),
    last_seen_at = COALESCE(last_seen_at, created_at);

CREATE INDEX idx_customer_source ON customer (source);
CREATE INDEX idx_customer_campaign ON customer (campaign);
CREATE INDEX idx_customer_lifecycle_stage ON customer (lifecycle_stage);
CREATE INDEX idx_customer_first_paid_at ON customer (first_paid_at);

CREATE TABLE bot_event
(
    id             BIGSERIAL PRIMARY KEY,
    customer_id    BIGINT REFERENCES customer (id) ON DELETE SET NULL,
    telegram_id    BIGINT,
    event_name     VARCHAR(80)              NOT NULL,
    source         VARCHAR(120),
    medium         VARCHAR(120),
    campaign       VARCHAR(120),
    stage          VARCHAR(80),
    amount         DECIMAL(20, 8),
    currency       VARCHAR(10),
    months         INTEGER,
    provider       VARCHAR(40),
    purchase_id    BIGINT REFERENCES purchase (id) ON DELETE SET NULL,
    metadata       JSONB                    NOT NULL DEFAULT '{}'::jsonb,
    created_at     TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_bot_event_customer_id ON bot_event (customer_id);
CREATE INDEX idx_bot_event_telegram_id ON bot_event (telegram_id);
CREATE INDEX idx_bot_event_event_name ON bot_event (event_name);
CREATE INDEX idx_bot_event_created_at ON bot_event (created_at);
CREATE INDEX idx_bot_event_event_name_created_at ON bot_event (event_name, created_at);
CREATE INDEX idx_bot_event_customer_id_created_at ON bot_event (customer_id, created_at);
CREATE INDEX idx_bot_event_source_created_at ON bot_event (source, created_at);
CREATE INDEX idx_bot_event_campaign_created_at ON bot_event (campaign, created_at);
CREATE INDEX idx_bot_event_purchase_id ON bot_event (purchase_id);
CREATE INDEX idx_bot_event_metadata ON bot_event USING GIN (metadata);

CREATE TABLE subscription_period
(
    id           BIGSERIAL PRIMARY KEY,
    customer_id  BIGINT REFERENCES customer (id) ON DELETE CASCADE,
    purchase_id  BIGINT REFERENCES purchase (id) ON DELETE SET NULL,
    source_type  VARCHAR(40)              NOT NULL,
    starts_at    TIMESTAMP WITH TIME ZONE NOT NULL,
    expires_at   TIMESTAMP WITH TIME ZONE NOT NULL,
    amount       DECIMAL(20, 8),
    currency     VARCHAR(10),
    months       INTEGER,
    provider     VARCHAR(40),
    metadata     JSONB                    NOT NULL DEFAULT '{}'::jsonb,
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_subscription_period_customer_id ON subscription_period (customer_id);
CREATE INDEX idx_subscription_period_purchase_id ON subscription_period (purchase_id);
CREATE INDEX idx_subscription_period_source_type ON subscription_period (source_type);
CREATE INDEX idx_subscription_period_starts_at ON subscription_period (starts_at);
CREATE INDEX idx_subscription_period_expires_at ON subscription_period (expires_at);

CREATE VIEW analytics_daily_funnel AS
SELECT
    date_trunc('day', created_at)::date AS day,
    COUNT(*) FILTER (WHERE event_name = 'start') AS starts,
    COUNT(*) FILTER (WHERE event_name = 'trial_activate') AS trials,
    COUNT(*) FILTER (WHERE event_name = 'buy_view') AS buy_views,
    COUNT(*) FILTER (WHERE event_name = 'plan_select') AS plan_selects,
    COUNT(*) FILTER (WHERE event_name = 'invoice_created') AS invoices,
    COUNT(*) FILTER (WHERE event_name = 'payment_success') AS payments,
    COUNT(DISTINCT customer_id) FILTER (WHERE event_name = 'start') AS unique_starts,
    COUNT(DISTINCT customer_id) FILTER (WHERE event_name = 'payment_success') AS unique_payers
FROM bot_event
GROUP BY 1;

CREATE VIEW analytics_customer_summary AS
WITH purchase_summary AS (
    SELECT
        customer_id,
        COUNT(*) FILTER (WHERE status = 'paid') AS paid_purchases,
        COALESCE(SUM(amount) FILTER (WHERE status = 'paid'), 0) AS lifetime_value,
        MAX(paid_at) AS last_paid_at
    FROM purchase
    GROUP BY customer_id
), period_summary AS (
    SELECT
        customer_id,
        MAX(expires_at) AS latest_subscription_expires_at
    FROM subscription_period
    GROUP BY customer_id
)
SELECT
    c.id AS customer_id,
    c.telegram_id,
    c.created_at,
    c.first_seen_at,
    c.last_seen_at,
    c.first_paid_at,
    c.source,
    c.medium,
    c.campaign,
    c.referrer_telegram_id,
    c.lifecycle_stage,
    c.lead_score,
    COALESCE(ps.paid_purchases, 0) AS paid_purchases,
    COALESCE(ps.lifetime_value, 0) AS lifetime_value,
    ps.last_paid_at,
    per.latest_subscription_expires_at
FROM customer c
LEFT JOIN purchase_summary ps ON ps.customer_id = c.id
LEFT JOIN period_summary per ON per.customer_id = c.id;

CREATE VIEW analytics_monthly_revenue AS
SELECT
    date_trunc('month', starts_at)::date AS month,
    COUNT(*) FILTER (WHERE source_type = 'paid') AS paid_periods,
    COALESCE(SUM(amount) FILTER (WHERE source_type = 'paid'), 0) AS revenue,
    COALESCE(SUM(amount / NULLIF(months, 0)) FILTER (WHERE source_type = 'paid'), 0) AS normalized_mrr
FROM subscription_period
GROUP BY 1;
