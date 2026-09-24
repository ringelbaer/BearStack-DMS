package photos

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// WalkExportMedia enumerates the gallery selection once, without pages, thumbnails,
// count queries or loading the entire collection. Existing query planners remain authoritative.
func (l *Library) WalkExportMedia(ctx context.Context, opts ListOptions, visit func(Media) error) error {
	rel, err := CleanPath(opts.Path)
	if err != nil {
		return err
	}
	plan := indexQueryPlanFor(opts.Query)
	indexed := l.index.available()
	person := IsPeopleFolder(rel)
	queryOpts := indexMediaOptions{Directory: rel, Subtree: true, Query: opts.Query, Plan: plan, MediaType: opts.MediaType, GPSOnly: opts.GPSOnly, IncludeAdminOnly: opts.IncludeAdminOnly}
	if person {
		check := opts
		check.SkipFolders = true
		check.SkipBlogs = true
		check.SkipMedia = true
		check.PageSize = 1
		if _, err = l.List(ctx, check); err != nil {
			return err
		}
		parts := strings.Split(rel, "/")
		if len(parts) != 3 {
			return os.ErrNotExist
		}
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return err
		}
		queryOpts.Directory = ""
		queryOpts.IncludeAdminOnly = false
		if strings.HasPrefix(parts[1], "f-") {
			raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(parts[1], "f-"))
			if err != nil {
				return err
			}
			queryOpts.Directory = string(raw)
		}
		condition := `mi.path IN (SELECT path FROM photo_faces WHERE person_id=? AND ignored=0)`
		if plan.ExpressionSQL != "" {
			condition = "(" + plan.ExpressionSQL + ") AND " + condition
		}
		plan.ExpressionSQL = condition
		plan.ExpressionArgs = append(plan.ExpressionArgs, id)
		queryOpts.Plan = plan
	} else {
		if opts.Query != "" {
			rel = ""
			queryOpts.Directory = ""
		}
		state, err := l.indexedListingPathState(ctx, rel, opts.IncludeAdminOnly, opts.Query != "" || rel != "")
		if err != nil {
			return err
		}
		indexed = indexed && state.Covered
	}
	if indexed {
		if queryHasPerson(opts.Query) {
			if err = l.refreshPeopleVisibility(ctx, ""); err != nil {
				return err
			}
		}
		// Complex search keeps the same bounded post-filter and face matching as the gallery.
		if plan.PostFilter {
			queryOpts.Limit = indexPostFilterCandidateMax
			queryOpts.LeanMetadata = true
			items, _, err := l.indexMedia(ctx, queryOpts)
			if err != nil {
				return err
			}
			for _, item := range items {
				if err = visit(item); err != nil {
					return err
				}
			}
			return nil
		}
		where, args, join := indexWhere(queryOpts)
		rows, err := l.index.db.QueryContext(ctx, "SELECT "+mediaIndexColumnsWithMetadata("mi", false)+" FROM "+indexMediaFrom(queryOpts, join)+where+" ORDER BY mi.path", args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			item, err := scanIndexedMediaWithMetadata(rows, false)
			if err != nil {
				return err
			}
			if err = visit(item); err != nil {
				return err
			}
		}
		return rows.Err()
	}
	abs, err := l.Resolve(rel)
	if err != nil {
		return err
	}
	query := compileMediaQuery(opts.Query)
	return walkPhotoFilesystem(ctx, photoFilesystemWalkOptions{Root: abs, StrictErrors: true, IncludeAdminOnly: opts.IncludeAdminOnly}, func(name string, entry os.DirEntry, kind string) error {
		if !isMediaKind(kind) {
			return nil
		}
		relative, err := filepath.Rel(l.root, name)
		if err != nil {
			return err
		}
		item, _, err := l.mediaFromPathData(filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		if len(filterMedia([]Media{item}, opts)) == 0 || !query.matches(item) {
			return nil
		}
		visible, err := l.filterImageGroupMembers(ctx, []Media{item})
		if err != nil {
			return err
		}
		if len(visible) == 0 {
			return nil
		}
		return visit(item)
	})
}

// WalkExportSelection preserves an explicit selection without expanding image groups.
// Batch reads and deduplication share the existing media and visibility rules.
func (l *Library) WalkExportSelection(ctx context.Context, paths []string, visit func(Media) error) error {
	seen := make(map[string]bool, len(paths))
	for _, name := range paths {
		for _, part := range strings.Split(name, "/") {
			if ignoredName(part) {
				return os.ErrNotExist
			}
		}
	}
	for start := 0; start < len(paths); start += 200 {
		items, err := l.MediaBatchContext(ctx, paths[start:min(start+200, len(paths))])
		if err != nil {
			return err
		}
		for _, item := range items {
			if seen[item.Path] || item.ImageGroupHidden {
				continue
			}
			seen[item.Path] = true
			if err = visit(item); err != nil {
				return err
			}
		}
	}
	return nil
}
