-- +goose Up
CREATE TABLE leads (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text NOT NULL,
    company         text,
    email           text NOT NULL,
    whatsapp        text NOT NULL,
    service_slug    text,
    start_timeframe text,
    message         text NOT NULL,
    status          text NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'contacted', 'done')),
    notes           text,
    ip_hash         text,
    user_agent      text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX leads_created_idx ON leads (created_at DESC);
CREATE INDEX leads_status_created_idx ON leads (status, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS leads;
