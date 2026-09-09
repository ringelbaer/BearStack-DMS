// Datei enthaelt den Einstieg fuer indexbasierte Foto-Listings.
package photos

import "context"

func (l *Library) listFromIndex(ctx context.Context, rel string, opts ListOptions, listing *Listing) (bool, error) {
	if l == nil || !l.index.available() {
		return false, nil
	}
	finishState := StartListTraceStep(ctx, "photos.index.path_state", ListTraceString("path", rel))
	state, err := l.indexedListingPathState(ctx, rel, opts.IncludeAdminOnly, opts.Query != "" || rel != "")
	finishState(
		ListTraceBool("covered", state.Covered),
		ListTraceString("order", state.Order),
	)
	if err != nil || !state.Covered {
		return false, err
	}
	order := state.Order
	listing.Order = order
	if opts.Query == "" {
		if !opts.Recursive && !opts.SkipFolders {
			finishFolders := StartListTraceStep(ctx, "photos.index.folders", ListTraceString("path", rel))
			listing.Folders, err = l.indexFolders(ctx, rel, opts.IncludeAdminOnly)
			finishFolders(ListTraceInt("count", len(listing.Folders)))
			if err != nil {
				return true, err
			}
		}
		if !opts.Recursive && !opts.SkipBlogs {
			err = l.indexListingBlogs(ctx, rel, opts, listing)
			if err != nil {
				return true, err
			}
		}
		if opts.SkipMedia {
			return true, nil
		}
		finishMedia := StartListTraceStep(ctx, "photos.index.media", ListTraceString("path", rel), ListTraceBool("subtree", opts.Recursive))
		listing.Media, listing.Total, err = l.indexMedia(ctx, indexMediaOptions{
			Directory:        rel,
			ExactDir:         !opts.Recursive,
			Subtree:          opts.Recursive,
			MediaType:        opts.MediaType,
			GPSOnly:          opts.GPSOnly,
			Order:            order,
			RequestSort:      opts.Sort,
			Limit:            opts.PageSize,
			Offset:           (opts.Page - 1) * opts.PageSize,
			LeanMetadata:     opts.LeanMetadata,
			IncludeAdminOnly: opts.IncludeAdminOnly,
		})
		finishMedia(ListTraceInt("count", len(listing.Media)), ListTraceInt("total", listing.Total))
		return true, err
	}
	finishPlan := StartListTraceStep(ctx, "photos.index.search_plan", ListTraceString("query", opts.Query))
	plan := indexQueryPlanFor(opts.Query)
	finishPlan(
		ListTraceBool("post_filter", plan.PostFilter),
		ListTraceBool("disjunctive", plan.Disjunctive),
		ListTraceBool("fts", plan.FTSQuery != ""),
		ListTraceInt("sql_terms", len(plan.SQLTerms)),
	)
	if !opts.SkipFolders {
		listing.Folders, err = l.indexSearchFolders(ctx, rel, opts.Query, opts.IncludeAdminOnly)
		if err != nil {
			return true, err
		}
	}
	if !opts.SkipBlogs {
		if err = l.indexListingBlogs(ctx, rel, opts, listing); err != nil {
			return true, err
		}
	}
	if opts.SkipMedia {
		return true, nil
	}
	finishMedia := StartListTraceStep(ctx, "photos.index.search_media", ListTraceString("path", rel))
	listing.Media, listing.Total, err = l.indexMedia(ctx, indexMediaOptions{
		Directory:        rel,
		Subtree:          true,
		Query:            opts.Query,
		Plan:             plan,
		MediaType:        opts.MediaType,
		GPSOnly:          opts.GPSOnly,
		Order:            order,
		RequestSort:      opts.Sort,
		Limit:            opts.PageSize,
		Offset:           (opts.Page - 1) * opts.PageSize,
		LeanMetadata:     opts.LeanMetadata,
		IncludeAdminOnly: opts.IncludeAdminOnly,
	})
	finishMedia(ListTraceInt("count", len(listing.Media)), ListTraceInt("total", listing.Total))
	return true, err
}
