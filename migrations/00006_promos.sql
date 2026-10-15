-- +goose Up
CREATE TABLE promos (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id  uuid REFERENCES services (id) ON DELETE SET NULL,
    title       text NOT NULL CHECK (title <> ''),
    description text NOT NULL DEFAULT '',
    code        text,
    starts_at   timestamptz NOT NULL,
    ends_at     timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at)
);
CREATE INDEX promos_ends_idx ON promos (ends_at);
CREATE INDEX promos_starts_idx ON promos (starts_at DESC);

-- +goose Down
DROP TABLE IF EXISTS promos;
