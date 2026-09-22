package photos

import "context"

// Only ordinary name-sorted listings with complete visibility counts are paged
// here. Search matchers, date ordering and incomplete indexes retain the shared
// fallback, which must see all candidates before it can select a correct page.
func (l *Library) indexFolderPage(ctx context.Context, rel string, opts ListOptions, listing *Listing) (bool, error) {
	order := opts.Sort
	if order == "" {
		order = listing.Order
	}
	if opts.FolderPageSize <= 0 || opts.Query != "" || opts.Recursive ||
		(order != "ascending_name" && order != "descending_name") {
		return false, nil
	}
	finish := StartListTraceStep(ctx, "photos.index.folder_page")
	defer func() { finish(ListTraceInt("returned", len(listing.Folders))) }()
	where := "parent = ?"
	visible := "1"
	unknown := "0"
	if !opts.IncludeAdminOnly {
		where += " AND admin_only = 0"
		visible = "public_recursive_media_count > 0 OR public_recursive_blog_count > 0 OR (recursive_media_count = 0 AND recursive_blog_count = 0)"
		unknown = "public_media_count < 0 OR public_recursive_media_count < 0 OR public_recursive_blog_count < 0"
	}
	// Keep eligibility, visibility and count in one read snapshot with the page.
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var total, incomplete int
	err = tx.QueryRowContext(ctx, `SELECT coalesce(sum(CASE WHEN (`+visible+`) THEN 1 ELSE 0 END),0),
  coalesce(max(`+unknown+`),0) FROM folder_index WHERE `+where, rel).Scan(&total, &incomplete)
	if err != nil {
		return false, err
	}
	if incomplete != 0 {
		return false, nil
	}
	offset, limit := pageOffset(opts.Page, opts.FolderPageSize), opts.FolderPageSize
	folders := []Folder{}
	if opts.IncludePeopleFolders && rel == "" {
		total++
		if offset == 0 {
			folders = append(folders, Folder{Name: "Personen", DisplayName: "Personen", Path: PeopleFolderPath, Virtual: true})
			limit--
		} else {
			offset--
		}
	}
	direction := "ASC"
	if order == "descending_name" {
		direction = "DESC"
	}
	if pageOffset(opts.Page, opts.FolderPageSize) >= total {
		limit = 0
	}
	if limit > 0 {
		rows, err := tx.QueryContext(ctx, `SELECT name,path,media_count,public_media_count,recursive_media_count,
   public_recursive_media_count,recursive_blog_count,public_recursive_blog_count,dir_count,mod_time_unix_nano,tags,admin_only
   FROM folder_index WHERE `+where+` AND (`+visible+`)
   ORDER BY bearstack_folder_name_key(name) `+direction+`,path `+direction+` LIMIT ? OFFSET ?`, rel, limit, offset)
		if err != nil {
			return false, err
		}
		for rows.Next() {
			scanned, err := scanIndexedFolder(rows, opts.IncludeAdminOnly)
			if err != nil {
				rows.Close()
				return false, err
			}
			folders = append(folders, scanned.folder)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	listing.Folders, listing.FolderTotal = folders, total
	listing.FolderHasNext = pageHasNext(opts.Page, opts.FolderPageSize, total)
	return true, nil
}
