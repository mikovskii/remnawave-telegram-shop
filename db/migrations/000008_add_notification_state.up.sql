ALTER TABLE purchase
    ADD COLUMN abandoned_claimed_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN abandoned_notified_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN failed_claimed_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN failed_notified_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN fulfilled_at TIMESTAMP WITH TIME ZONE;

CREATE INDEX idx_purchase_abandoned_candidates
    ON purchase (status, created_at)
    WHERE abandoned_notified_at IS NULL;

CREATE INDEX idx_purchase_unfulfilled_paid
    ON purchase (status, fulfilled_at)
    WHERE status = 'paid' AND fulfilled_at IS NULL;

CREATE TABLE notification_log
(
    id          BIGSERIAL PRIMARY KEY,
    customer_id BIGINT REFERENCES customer (id) ON DELETE CASCADE,
    type        VARCHAR(80)              NOT NULL,
    dedupe_key  VARCHAR(160)             NOT NULL,
    status      VARCHAR(20)              NOT NULL DEFAULT 'sent',
    metadata    JSONB                    NOT NULL DEFAULT '{}'::jsonb,
    sent_at     TIMESTAMP WITH TIME ZONE,
    last_error  TEXT,
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (customer_id, type, dedupe_key)
);

CREATE INDEX idx_notification_log_customer_id ON notification_log (customer_id);
CREATE INDEX idx_notification_log_type_created_at ON notification_log (type, created_at);
CREATE INDEX idx_notification_log_status ON notification_log (status);
