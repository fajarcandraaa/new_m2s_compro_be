-- +goose Up
CREATE TABLE testimonials (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    client_name text NOT NULL CHECK (client_name <> ''),
    client_role text NOT NULL DEFAULT '',
    company     text,
    quote       text NOT NULL CHECK (quote <> ''),
    is_active   boolean NOT NULL DEFAULT true,
    sort_order  integer NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX testimonials_sort_idx ON testimonials (sort_order);

CREATE TABLE client_logos (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL CHECK (name <> ''),
    image_url  text NOT NULL,
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX client_logos_sort_idx ON client_logos (sort_order);

-- +goose Down
DROP TABLE IF EXISTS client_logos;
DROP TABLE IF EXISTS testimonials;
