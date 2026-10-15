-- +goose Up
CREATE TABLE settings (
    id                smallint PRIMARY KEY CHECK (id = 1),
    company_name      text NOT NULL,
    legal_name        text NOT NULL,
    tagline           text NOT NULL DEFAULT '',
    email             text NOT NULL,
    whatsapp_number   text NOT NULL CHECK (whatsapp_number ~ '^\+[1-9][0-9]{7,14}$'),
    whatsapp_display  text NOT NULL DEFAULT '',
    whatsapp_greeting text NOT NULL DEFAULT '',
    address           text NOT NULL DEFAULT '',
    maps_url          text,
    linkedin_url      text,
    instagram_url     text,
    show_testimonials boolean NOT NULL DEFAULT true,
    show_client_logos boolean NOT NULL DEFAULT false,
    updated_at        timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS settings;
