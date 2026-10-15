package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fajarcandraaa/new_m2s_compro_be/internal/domain"
)

const articleCols = `a.id, a.slug, a.status, a.title, a.excerpt, a.content_html, a.cover_image_url,
	a.author, a.read_minutes, a.published_at, a.seo_title, a.seo_description, a.og_image_url,
	(a.status = 'published' AND a.published_at > now()) AS is_scheduled, a.created_at, a.updated_at`

// visibleArticle is the public visibility condition.
const visibleArticle = "a.status = 'published' AND a.published_at <= now()"

func scanArticle(row pgx.Row) (domain.Article, error) {
	var v domain.Article
	err := row.Scan(&v.ID, &v.Slug, &v.Status, &v.Title, &v.Excerpt, &v.ContentHTML, &v.CoverImageURL,
		&v.Author, &v.ReadMinutes, &v.PublishedAt, &v.Seo.Title, &v.Seo.Description, &v.Seo.OgImageURL,
		&v.IsScheduled, &v.CreatedAt, &v.UpdatedAt)
	v.Tags = []domain.Tag{}
	return v, err
}

func (s *Store) collectArticles(ctx context.Context, rows pgx.Rows) ([]domain.Article, error) {
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Article, error) { return scanArticle(r) })
	if err != nil {
		return nil, fmt.Errorf("store: articles: %w", err)
	}
	if err := s.attachTags(ctx, list); err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Store) attachTags(ctx context.Context, list []domain.Article) error {
	if len(list) == 0 {
		return nil
	}
	ids := make([]string, len(list))
	idx := make(map[string]int, len(list))
	for i, v := range list {
		ids[i] = v.ID
		idx[v.ID] = i
	}
	rows, err := s.db.Query(ctx, `SELECT at.article_id::text, t.slug, t.name FROM article_tags at
		JOIN tags t ON t.id = at.tag_id WHERE at.article_id = ANY($1::uuid[])
		ORDER BY at.article_id, at.position`, ids)
	if err != nil {
		return fmt.Errorf("store: article tags: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var aid string
		var t domain.Tag
		if err := rows.Scan(&aid, &t.Slug, &t.Name); err != nil {
			return fmt.Errorf("store: article tags scan: %w", err)
		}
		i := idx[aid]
		list[i].Tags = append(list[i].Tags, t)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: article tags rows: %w", err)
	}
	return nil
}

func tagFilter(w *where, tagSlug string) {
	w.add(`EXISTS (SELECT 1 FROM article_tags ft JOIN tags tt ON tt.id = ft.tag_id
		WHERE ft.article_id = a.id AND tt.slug = $?)`, tagSlug)
}

// ListPublicArticles returns visible articles by published_at desc.
func (s *Store) ListPublicArticles(ctx context.Context, tagSlug string, p Page) ([]domain.Article, int, error) {
	var w where
	w.add(visibleArticle)
	if tagSlug != "" {
		tagFilter(&w, tagSlug)
	}
	var total int
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM articles a"+w.sql(), w.args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count articles: %w", err)
	}
	limit := w.next(p.Size)
	offset := w.next(p.offset())
	rows, err := s.db.Query(ctx, "SELECT "+articleCols+" FROM articles a"+w.sql()+
		" ORDER BY a.published_at DESC, a.id LIMIT "+limit+" OFFSET "+offset, w.args...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list articles: %w", err)
	}
	list, err := s.collectArticles(ctx, rows)
	return list, total, err
}

// GetVisibleArticleBySlug returns a publicly visible article.
func (s *Store) GetVisibleArticleBySlug(ctx context.Context, slug string) (*domain.Article, error) {
	rows, err := s.db.Query(ctx, "SELECT "+articleCols+" FROM articles a WHERE a.slug = $1 AND "+visibleArticle, slug)
	if err != nil {
		return nil, fmt.Errorf("store: get article: %w", err)
	}
	list, err := s.collectArticles(ctx, rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("store: get article %s: %w", slug, pgx.ErrNoRows)
	}
	return &list[0], nil
}

// ListRelatedArticles returns visible articles sharing >= 1 tag with the
// article, by shared tag count desc then published_at desc.
func (s *Store) ListRelatedArticles(ctx context.Context, articleID string, limit int) ([]domain.Article, error) {
	rows, err := s.db.Query(ctx, `SELECT `+articleCols+` FROM articles a
		JOIN (SELECT t2.article_id, count(*) AS shared FROM article_tags t1
			JOIN article_tags t2 ON t2.tag_id = t1.tag_id AND t2.article_id <> t1.article_id
			WHERE t1.article_id = $1 GROUP BY t2.article_id) r ON r.article_id = a.id
		WHERE `+visibleArticle+`
		ORDER BY r.shared DESC, a.published_at DESC, a.id LIMIT $2`, articleID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: related articles: %w", err)
	}
	return s.collectArticles(ctx, rows)
}

// AdminArticleFilter filters the CMS table.
type AdminArticleFilter struct {
	Q       string
	Status  string // draft | published (visible now) | scheduled
	TagSlug string
	Page    Page
}

// ListAdminArticles returns a page ordered by updated_at desc.
func (s *Store) ListAdminArticles(ctx context.Context, f AdminArticleFilter) ([]domain.Article, int, error) {
	var w where
	if f.Q != "" {
		w.add("(a.title ILIKE $? OR a.slug ILIKE $?)", likePattern(f.Q))
	}
	switch f.Status {
	case "draft":
		w.add("a.status = 'draft'")
	case "published":
		w.add(visibleArticle)
	case "scheduled":
		w.add("a.status = 'published' AND a.published_at > now()")
	}
	if f.TagSlug != "" {
		tagFilter(&w, f.TagSlug)
	}
	var total int
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM articles a"+w.sql(), w.args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count articles: %w", err)
	}
	limit := w.next(f.Page.Size)
	offset := w.next(f.Page.offset())
	rows, err := s.db.Query(ctx, "SELECT "+articleCols+" FROM articles a"+w.sql()+
		" ORDER BY a.updated_at DESC, a.id LIMIT "+limit+" OFFSET "+offset, w.args...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list articles: %w", err)
	}
	list, err := s.collectArticles(ctx, rows)
	return list, total, err
}

// GetArticle returns an article by id (any status).
func (s *Store) GetArticle(ctx context.Context, id string) (*domain.Article, error) {
	rows, err := s.db.Query(ctx, "SELECT "+articleCols+" FROM articles a WHERE a.id = $1", id)
	if err != nil {
		return nil, fmt.Errorf("store: get article: %w", err)
	}
	list, err := s.collectArticles(ctx, rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("store: get article %s: %w", id, pgx.ErrNoRows)
	}
	return &list[0], nil
}

// SaveArticle creates (id == "") or fully updates an article and replaces
// its tags. tagNames are already deduplicated by slug, in display order.
func (s *Store) SaveArticle(ctx context.Context, id string, v *domain.Article) (*domain.Article, error) {
	var savedID string
	err := s.Tx(ctx, func(tx *Store) error {
		slug, err := tx.resolveSlug(ctx, "articles", id, v.Slug, v.Title)
		if err != nil {
			return err
		}
		args := []any{slug, v.Status, v.Title, v.Excerpt, v.ContentHTML, v.CoverImageURL, v.Author,
			v.ReadMinutes, v.PublishedAt, v.Seo.Title, v.Seo.Description, v.Seo.OgImageURL}
		if id == "" {
			err = tx.db.QueryRow(ctx, `INSERT INTO articles (slug, status, title, excerpt, content_html,
				cover_image_url, author, read_minutes, published_at, seo_title, seo_description, og_image_url)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id::text`, args...).Scan(&savedID)
			if err != nil {
				return fmt.Errorf("store: insert article: %w", err)
			}
		} else {
			args = append(args, id)
			tag, err := tx.db.Exec(ctx, `UPDATE articles SET slug = $1, status = $2, title = $3,
				excerpt = $4, content_html = $5, cover_image_url = $6, author = $7, read_minutes = $8,
				published_at = $9, seo_title = $10, seo_description = $11, og_image_url = $12,
				updated_at = now() WHERE id = $13`, args...)
			if err != nil {
				return fmt.Errorf("store: update article: %w", err)
			}
			if tag.RowsAffected() == 0 {
				return fmt.Errorf("store: update article: %w", pgx.ErrNoRows)
			}
			savedID = id
		}
		return tx.setArticleTags(ctx, savedID, v.Tags)
	})
	if err != nil {
		return nil, err
	}
	return s.GetArticle(ctx, savedID)
}

// setArticleTags resolves tags by slug (creating missing ones; an existing
// tag keeps its name) and replaces the article's tag rows with positions.
func (s *Store) setArticleTags(ctx context.Context, articleID string, tags []domain.Tag) error {
	if _, err := s.db.Exec(ctx, "DELETE FROM article_tags WHERE article_id = $1", articleID); err != nil {
		return fmt.Errorf("store: clear article tags: %w", err)
	}
	if len(tags) == 0 {
		return nil
	}
	slugs := make([]string, len(tags))
	names := make([]string, len(tags))
	for i, t := range tags {
		slugs[i] = t.Slug
		names[i] = t.Name
	}
	// Slugs are derived from lowercased names, so the slug and lower(name)
	// unique indexes conflict together; an existing tag keeps its name.
	if _, err := s.db.Exec(ctx, `INSERT INTO tags (slug, name)
		SELECT x.slug, x.name FROM unnest($1::text[], $2::text[]) AS x(slug, name)
		ON CONFLICT DO NOTHING`, slugs, names); err != nil {
		return fmt.Errorf("store: insert tags: %w", err)
	}
	_, err := s.db.Exec(ctx, `INSERT INTO article_tags (article_id, tag_id, position)
		SELECT $1, t.id, min(x.pos - 1) FROM unnest($2::text[], $3::text[]) WITH ORDINALITY AS x(slug, name, pos)
		JOIN tags t ON t.slug = x.slug
		GROUP BY t.id`, articleID, slugs, names)
	if err != nil {
		return fmt.Errorf("store: insert article tags: %w", err)
	}
	return nil
}

// DeleteArticle deletes an article.
func (s *Store) DeleteArticle(ctx context.Context, id string) error {
	return s.deleteByID(ctx, "articles", id)
}

// ListTags returns all tags by name with the number of visible articles.
func (s *Store) ListTags(ctx context.Context) ([]domain.TagWithCount, error) {
	rows, err := s.db.Query(ctx, `SELECT t.slug, t.name,
		(SELECT count(*) FROM article_tags at JOIN articles a ON a.id = at.article_id
		 WHERE at.tag_id = t.id AND `+visibleArticle+`) AS n
		FROM tags t ORDER BY lower(t.name), t.slug`)
	if err != nil {
		return nil, fmt.Errorf("store: list tags: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.TagWithCount, error) {
		var t domain.TagWithCount
		err := r.Scan(&t.Slug, &t.Name, &t.ArticleCount)
		return t, err
	})
	if err != nil {
		return nil, fmt.Errorf("store: list tags: %w", err)
	}
	return list, nil
}

// InsertTags inserts tags that do not exist yet (seed).
func (s *Store) InsertTags(ctx context.Context, tags []domain.Tag) error {
	for _, t := range tags {
		if _, err := s.db.Exec(ctx, "INSERT INTO tags (slug, name) VALUES ($1, $2) ON CONFLICT DO NOTHING", t.Slug, t.Name); err != nil {
			return fmt.Errorf("store: insert tag: %w", err)
		}
	}
	return nil
}
