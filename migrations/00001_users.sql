-- +goose Up
CREATE TABLE users (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text NOT NULL CHECK (name <> ''),
    email           text NOT NULL,
    password_hash   text NOT NULL,
    role            text NOT NULL CHECK (role IN ('admin', 'editor')),
    is_active       boolean NOT NULL DEFAULT true,
    session_version integer NOT NULL DEFAULT 1,
    last_login_at   timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_lower_uq ON users (lower(email));

-- +goose Down
DROP TABLE IF EXISTS users;
