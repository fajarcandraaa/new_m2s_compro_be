-- +goose Up
CREATE TABLE case_studies (
    id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug                     text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    status                   text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
    system_name              text NOT NULL,
    category                 text NOT NULL,
    featured                 boolean NOT NULL DEFAULT false,
    sort_order               integer NOT NULL DEFAULT 0,
    title                    text NOT NULL CHECK (title <> ''),
    summary                  text NOT NULL DEFAULT '',
    card_body                text NOT NULL DEFAULT '',
    metrics                  jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(metrics) = 'array'),
    scope                    jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(scope) = 'array'),
    initial_conditions       jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(initial_conditions) = 'array'),
    risks                    jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(risks) = 'array'),
    approach                 jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(approach) = 'array'),
    solutions                jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(solutions) = 'array'),
    impacts                  jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(impacts) = 'array'),
    initial_conditions_title text NOT NULL DEFAULT '',
    risks_title              text NOT NULL DEFAULT '',
    approach_title           text NOT NULL DEFAULT '',
    impact_title             text NOT NULL DEFAULT '',
    lesson                   text NOT NULL DEFAULT '',
    seo_title                text,
    seo_description          text,
    og_image_url             text,
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX case_studies_status_sort_idx ON case_studies (status, sort_order);

-- +goose Down
DROP TABLE IF EXISTS case_studies;
