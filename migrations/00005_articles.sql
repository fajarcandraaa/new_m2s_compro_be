-- +goose Up
CREATE TABLE tags (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug       text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX tags_name_lower_uq ON tags (lower(name));

CREATE TABLE articles (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    status          text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
    title           text NOT NULL CHECK (title <> ''),
    excerpt         text NOT NULL DEFAULT '',
    content_html    text NOT NULL DEFAULT '',
    cover_image_url text,
    author          text NOT NULL DEFAULT 'Tim Mind2Screen',
    read_minutes    integer NOT NULL DEFAULT 1 CHECK (read_minutes >= 1),
    published_at    timestamptz,
    seo_title       text,
    seo_description text,
    og_image_url    text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CHECK (status = 'draft' OR published_at IS NOT NULL)
);
CREATE INDEX articles_public_idx ON articles (published_at DESC) WHERE status = 'published';
CREATE INDEX articles_updated_idx ON articles (updated_at DESC);

CREATE TABLE article_tags (
    article_id uuid NOT NULL REFERENCES articles (id) ON DELETE CASCADE,
    tag_id     uuid NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    position   smallint NOT NULL DEFAULT 0,
    PRIMARY KEY (article_id, tag_id)
);
CREATE INDEX article_tags_tag_idx ON article_tags (tag_id, article_id);

-- +goose Down
DROP TABLE IF EXISTS article_tags;
DROP TABLE IF EXISTS articles;
DROP TABLE IF EXISTS tags;
