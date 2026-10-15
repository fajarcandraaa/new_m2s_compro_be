-- +goose Up
-- FAQ items ([{question, answer}], max 10) for FAQPage JSON-LD (contract v1.1).
ALTER TABLE services ADD COLUMN faqs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(faqs) = 'array');
ALTER TABLE articles ADD COLUMN faqs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(faqs) = 'array');

-- +goose Down
ALTER TABLE articles DROP COLUMN IF EXISTS faqs;
ALTER TABLE services DROP COLUMN IF EXISTS faqs;
