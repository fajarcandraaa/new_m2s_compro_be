-- +goose Up
CREATE TABLE services (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    audience        text NOT NULL CHECK (audience IN ('business', 'personal')),
    status          text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
    sort_order      integer NOT NULL DEFAULT 0,
    eyebrow         text NOT NULL DEFAULT '',
    title           text NOT NULL CHECK (title <> ''),
    tagline         text NOT NULL DEFAULT '',
    summary         text NOT NULL DEFAULT '',
    price_label     text NOT NULL DEFAULT '',
    price_value     text NOT NULL DEFAULT '',
    audience_text   text NOT NULL DEFAULT '',
    points          jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(points) = 'array'),
    deliverables    jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(deliverables) = 'array'),
    systems         jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(systems) = 'array'),
    seo_title       text,
    seo_description text,
    og_image_url    text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX services_status_sort_idx ON services (status, sort_order);

CREATE TABLE service_packages (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id  uuid NOT NULL REFERENCES services (id) ON DELETE CASCADE,
    name        text NOT NULL CHECK (name <> ''),
    price_label text NOT NULL DEFAULT '',
    price_value text NOT NULL,
    description text NOT NULL DEFAULT '',
    features    jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(features) = 'array'),
    highlighted boolean NOT NULL DEFAULT false,
    sort_order  integer NOT NULL DEFAULT 0
);
CREATE INDEX service_packages_service_idx ON service_packages (service_id, sort_order);

-- +goose Down
DROP TABLE IF EXISTS service_packages;
DROP TABLE IF EXISTS services;
