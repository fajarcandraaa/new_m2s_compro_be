// Package compro holds the files embedded into the backend binary: SQL
// migrations and the seed content.
package compro

import "embed"

// Migrations contains the goose SQL migrations (migrations/*.sql).
//
//go:embed migrations/*.sql
var Migrations embed.FS

// Content contains the seed content (docs/content/*.json).
//
//go:embed docs/content/seed.json docs/content/articles.json
var Content embed.FS
