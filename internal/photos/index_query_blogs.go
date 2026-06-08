// Datei enthaelt Blogabfragen aus dem Fotoindex.
package photos

import (
	"context"
	"strings"
	"time"

	"bearstack/internal/searchtext"
)

func (l *Library) indexBlogs(ctx context.Context, rel, query string, includeAdminOnly bool) ([]BlogPost, error) {
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
	sql := `SELECT bi.path, bi.name, bi.date, bi.mod_time_unix_nano, bi.text, bi.tags, bi.admin_only FROM ` + from
	if len(where) > 0 {
		sql += ` WHERE ` + strings.Join(where, " AND ")
	}
	sql += ` ORDER BY bi.date DESC, bi.mod_time_unix_nano DESC, bi.path DESC`
	if !postFilter {
		sql += ` LIMIT 50`
	}
	rows, err := l.index.db.QueryContext(ctx, sql, args...)
	if err != nil {
		finishTrace(ListTraceString("error", err.Error()))
		return nil, err
	}
	defer rows.Close()
	blogs := make([]BlogPost, 0)
	for rows.Next() {
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
		post.HTML = renderMarkdown([]byte(post.Text))
		if query == "" || matchesBlogQuery(post, query) {
			blogs = append(blogs, post)
		}
	}
	if err := rows.Err(); err != nil {
		finishTrace(ListTraceString("error", err.Error()))
		return nil, err
	}
	if postFilter && len(blogs) > 50 {
		blogs = blogs[:50]
	}
	finishTrace(
		ListTraceInt("count", len(blogs)),
		ListTraceBool("join_search", joinSearch),
		ListTraceBool("post_filter", postFilter),
	)
	return blogs, nil
}
