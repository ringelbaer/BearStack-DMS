// Datei enthaelt Blogabfragen aus dem Fotoindex.
package photos

import (
	"context"
	"strings"
	"time"

	"bearstack/internal/searchtext"
)

func (l *Library) indexBlogs(ctx context.Context, rel, query string, includeAdminOnly bool) ([]BlogPost, error) {
	return l.indexBlogsPage(ctx, rel, query, includeAdminOnly, 0, 50, false)
}

func (l *Library) indexListingBlogs(ctx context.Context, rel string, opts ListOptions, listing *Listing) error {
	if opts.BlogPageSize <= 0 {
		var err error
		listing.Blogs, err = l.indexBlogs(ctx, rel, opts.Query, opts.IncludeAdminOnly)
		return err
	}
	posts, err := l.indexBlogsPage(ctx, rel, opts.Query, opts.IncludeAdminOnly,
		(opts.Page-1)*opts.BlogPageSize, opts.BlogPageSize+1, opts.BlogSummaries)
	if err != nil {
		return err
	}
	listing.BlogHasNext = len(posts) > opts.BlogPageSize
	listing.Blogs = posts[:min(len(posts), opts.BlogPageSize)]
	return nil
}

func (l *Library) indexBlogsPage(ctx context.Context, rel, query string, includeAdminOnly bool, offset, limit int, summaries bool) ([]BlogPost, error) {
	if l == nil || !l.index.available() {
		return nil, nil
	}
	finishTrace := StartListTraceStep(ctx, "photos.index.blog_query", ListTraceString("path", rel), ListTraceBool("search", query != ""))
	where := []string{}
	args := []any{}
	joinSearch := false
	postFilter := false
	if query != "" {
		plan := indexQueryPlanFor(query)
		postFilter = plan.PostFilter
		joinSearch = !plan.Disjunctive && blogSearchNeedsFTS(plan)
		if joinSearch {
			fts := blogSearchFTSQuery(plan, query)
			if fts != "" {
				where = append(where, "blog_search MATCH ?")
				args = append(args, fts)
			} else {
				joinSearch = false
			}
		}
		for _, term := range plan.SQLTerms {
			switch term.Field {
			case "tag":
				for _, tag := range cleanPhotoTags([]string{term.Value}) {
					if term.Negated {
						where = append(where, "NOT EXISTS (SELECT 1 FROM blog_tag_index bti WHERE bti.blog_path = bi.path AND bti.tag = ?)")
					} else {
						where = append(where, "bi.path IN (SELECT blog_path FROM blog_tag_index WHERE tag = ?)")
					}
					args = append(args, tag)
				}
			case "directory":
				value := strings.Trim(strings.TrimSpace(term.Value), "/")
				if value != "" {
					where = append(where, "bearstack_german_fold(bi.directory) LIKE ? ESCAPE '\\'")
					args = append(args, searchtext.LikeContainsPattern(searchtext.GermanFold(value)))
				}
			case "file_name":
				where = append(where, "bearstack_german_fold(bi.name) LIKE ? ESCAPE '\\'")
				args = append(args, searchtext.LikeContainsPattern(searchtext.GermanFold(term.Value)))
			case "type":
				if !strings.EqualFold(term.Value, MediaTypeBlog) {
					finishTrace(ListTraceString("skipped", "type_filter"))
					return nil, nil
				}
			}
		}
	}
	if query == "" {
		where = append(where, "bi.directory = ?")
		args = append(args, rel)
	} else if rel != "" {
		start, end := prefixRange(rel + "/")
		where = append(where, "(bi.directory = ? OR (bi.directory >= ? AND bi.directory < ?))")
		args = append(args, rel, start, end)
	}
	if !includeAdminOnly {
		where = append(where, "bi.admin_only = 0")
	}
	from := "blog_index bi"
	if joinSearch {
		from += " JOIN blog_search ON blog_search.rowid = bi.rowid"
	}
	textColumn := "bi.text"
	if summaries && query == "" {
		textColumn = "''"
	}
	sql := `SELECT bi.path, bi.name, bi.date, bi.mod_time_unix_nano, ` + textColumn + `, bi.tags, bi.admin_only FROM ` + from
	if len(where) > 0 {
		sql += ` WHERE ` + strings.Join(where, " AND ")
	}
	sql += ` ORDER BY bi.date DESC, bi.mod_time_unix_nano DESC, bi.path DESC`
	// Search still passes through the shared matcher. Paginate matches, not candidates.
	if query == "" {
		sql += ` LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
		offset = 0
	}
	rows, err := l.index.db.QueryContext(ctx, sql, args...)
	if err != nil {
		finishTrace(ListTraceString("error", err.Error()))
		return nil, err
	}
	defer rows.Close()
	blogs := make([]BlogPost, 0)
	scanned := 0
	for rows.Next() {
		scanned++
		if query != "" && scanned > 10000 {
			return nil, ErrSearchTooBroad()
		}
		var post BlogPost
		var dateValue string
		var modUnix int64
		var rawTags string
		var adminOnly int
		if err := rows.Scan(&post.Path, &post.Name, &dateValue, &modUnix, &post.Text, &rawTags, &adminOnly); err != nil {
			return nil, err
		}
		post.ModTime = time.Unix(0, modUnix)
		post.Tags = tagsFromJSON(rawTags)
		post.AdminOnly = adminOnly != 0
		if dateValue != "" {
			if parsed, err := time.Parse("2006-01-02", dateValue); err == nil {
				post.Date = &parsed
			}
		}
		if query == "" || matchesBlogQuery(post, query) {
			if offset > 0 {
				offset--
				continue
			}
			if summaries {
				post.Text = ""
			} else {
				_, post.HTML = blogContent(post.Name, []byte(post.Text))
			}
			blogs = append(blogs, post)
			if len(blogs) >= limit {
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		finishTrace(ListTraceString("error", err.Error()))
		return nil, err
	}
	finishTrace(
		ListTraceInt("count", len(blogs)),
		ListTraceBool("join_search", joinSearch),
		ListTraceBool("post_filter", postFilter),
	)
	return blogs, nil
}
