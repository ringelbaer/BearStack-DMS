// Datei loest virtuelle BearStack-Ordner und Dokumente fuer WebDAV-Pfade auf.
package server

import (
	"context"
	"database/sql"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bearstack/internal/document"
)

type webDAVResource struct {
	Name      string
	Segments  []string
	IsDir     bool
	Selection virtualFolderSelection
	Document  document.Document
}

type webDAVResolver struct {
	server *Server
}

func (s *Server) webDAVResolver() webDAVResolver {
	return webDAVResolver{server: s}
}

func (resolver webDAVResolver) Resolve(ctx context.Context, segments []string) (webDAVResource, error) {
	if len(segments) == 0 {
		return webDAVResource{Name: "", Segments: nil, IsDir: true}, nil
	}

	if isSearchFavoritesFolderName(segments[0]) {
		return resolver.resolveSearchFavoriteResource(ctx, segments)
	}
	return resolver.resolveVirtualResource(ctx, segments)
}

func (resolver webDAVResolver) resolveSearchFavoriteResource(ctx context.Context, segments []string) (webDAVResource, error) {
	s := resolver.server
	if len(segments) == 1 {
		if _, err := s.folderService().SearchFavoriteItems(ctx, time.Now()); err != nil {
			return webDAVResource{}, err
		}
		return webDAVResource{Name: searchFavoritesFolderName, Segments: segments, IsDir: true}, nil
	}
	favoriteName, err := unescapePathComponent(segments[1])
	if err != nil {
		return webDAVResource{}, err
	}
	favorite, err := s.folderService().SearchFavorite(ctx, favoriteName)
	if err != nil {
		return webDAVResource{}, err
	}
	if len(segments) == 2 {
		return webDAVResource{Name: escapePathComponent(favorite.Name), Segments: segments, IsDir: true}, nil
	}
	if len(segments) > 3 {
		return webDAVResource{}, sql.ErrNoRows
	}

	filter := searchFavoriteFilter(favorite, time.Now(), 0, 0)
	docs, err := s.repo.ListDocuments(ctx, filter)
	if err != nil {
		return webDAVResource{}, err
	}
	for _, item := range namedDocuments(docs) {
		if item.Name == segments[2] {
			return webDAVResource{Name: item.Name, Segments: segments, Document: item.Document}, nil
		}
	}
	return webDAVResource{}, sql.ErrNoRows
}

func (resolver webDAVResolver) resolveVirtualResource(ctx context.Context, segments []string) (webDAVResource, error) {
	current := webDAVResource{Name: "", Segments: nil, IsDir: true}
	for i, segment := range segments {
		candidates := webDAVSegmentCandidates(segment)
		listing, err := resolver.Children(ctx, current)
		if err != nil {
			return webDAVResource{}, err
		}
		for _, child := range listing {
			if webDAVNameMatches(child.Name, candidates) && child.IsDir {
				if i == len(segments)-1 {
					return child, nil
				}
				current = child
				goto nextSegment
			}
		}
		if i == len(segments)-1 {
			for _, child := range listing {
				if webDAVNameMatches(child.Name, candidates) && !child.IsDir {
					return child, nil
				}
			}
		}
		return webDAVResource{}, sql.ErrNoRows
	nextSegment:
	}
	return webDAVResource{}, sql.ErrNoRows
}

func (resolver webDAVResolver) Children(ctx context.Context, resource webDAVResource) ([]webDAVResource, error) {
	if !resource.IsDir {
		return nil, nil
	}
	if len(resource.Segments) == 0 {
		return resolver.rootChildren(ctx)
	}
	if isSearchFavoritesFolderName(resource.Segments[0]) {
		return resolver.searchFavoriteChildren(ctx, resource)
	}

	return resolver.virtualChildren(ctx, resource)
}

func (resolver webDAVResolver) rootChildren(ctx context.Context) ([]webDAVResource, error) {
	s := resolver.server
	filter := document.ListFilter{
		Sort:      document.ListSortDate,
		Direction: document.ListDirectionDescending,
	}
	items, err := s.folderService().Items(ctx, virtualFolderSelection{}, filter)
	if err != nil {
		return nil, err
	}
	children, _ := webDAVResourcesFromFolderViewItems(items, nil)
	return children, nil
}

func (resolver webDAVResolver) virtualChildren(ctx context.Context, resource webDAVResource) ([]webDAVResource, error) {
	s := resolver.server
	selection := resource.Selection
	filter := document.ListFilter{
		Sort:      document.ListSortDate,
		Direction: document.ListDirectionDescending,
	}
	filter = selection.ApplyToFilter(filter)
	items, err := s.folderService().Items(ctx, selection, filter)
	if err != nil {
		return nil, err
	}
	parentSegments := append([]string(nil), resource.Segments...)
	children, reserved := webDAVResourcesFromFolderViewItems(items, parentSegments)

	if selection.Depth() == 0 {
		return children, nil
	}
	docs, err := s.repo.ListDocuments(ctx, filter)
	if err != nil {
		return nil, err
	}
	for _, item := range uniqueDocumentNames(docs, reserved) {
		children = append(children, webDAVResource{
			Name:     item.Name,
			Segments: append(append([]string(nil), parentSegments...), item.Name),
			Document: item.Document,
		})
	}
	return children, nil
}

func (resolver webDAVResolver) searchFavoriteChildren(ctx context.Context, resource webDAVResource) ([]webDAVResource, error) {
	s := resolver.server
	if len(resource.Segments) == 1 {
		items, err := s.folderService().SearchFavoriteItems(ctx, time.Now())
		if err != nil {
			return nil, err
		}
		children, _ := webDAVResourcesFromFolderViewItems(items, []string{searchFavoritesFolderName})
		return children, nil
	}
	if len(resource.Segments) != 2 {
		return nil, nil
	}
	favoriteName, err := unescapePathComponent(resource.Segments[1])
	if err != nil {
		return nil, err
	}
	filter, err := s.folderService().SearchFavoriteFilter(ctx, favoriteName, time.Now(), 0, 0)
	if err != nil {
		return nil, err
	}
	docs, err := s.repo.ListDocuments(ctx, filter)
	if err != nil {
		return nil, err
	}
	children := make([]webDAVResource, 0, len(docs))
	for _, item := range namedDocuments(docs) {
		children = append(children, webDAVResource{
			Name:     item.Name,
			Segments: []string{searchFavoritesFolderName, resource.Segments[1], item.Name},
			Document: item.Document,
		})
	}
	return children, nil
}

func webDAVResourcesFromFolderViewItems(items []folderViewItem, parentSegments []string) ([]webDAVResource, map[string]struct{}) {
	resources := make([]webDAVResource, 0, len(items))
	reserved := make(map[string]struct{}, len(items))
	for _, item := range items {
		name := escapePathComponent(item.Name)
		if item.Kind == folderViewKindFieldValue {
			if _, exists := reserved[strings.ToLower(name)]; exists {
				name = escapePathComponent(item.Name + " (" + strconv.FormatInt(item.FieldID, 10) + ")")
			}
		}
		reserved[strings.ToLower(name)] = struct{}{}
		resources = append(resources, webDAVResource{
			Name:      name,
			Segments:  append(append([]string(nil), parentSegments...), name),
			IsDir:     true,
			Selection: item.Selection,
		})
	}
	return resources, reserved
}

func webDAVNameMatches(name string, candidates []string) bool {
	for _, candidate := range candidates {
		if name == candidate {
			return true
		}
	}
	return false
}

func webDAVSegmentCandidates(segment string) []string {
	seen := map[string]struct{}{}
	candidates := make([]string, 0, 4)
	add := func(value string) {
		value = webDAVNormalizeSegment(value)
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		candidates = append(candidates, value)
	}

	add(segment)
	if strings.Contains(segment, "+") {
		add(strings.ReplaceAll(segment, "+", " "))
	}
	current := segment
	for i := 0; i < 2; i++ {
		decoded, err := url.PathUnescape(current)
		if err != nil || decoded == current {
			break
		}
		add(decoded)
		if strings.Contains(decoded, "+") {
			add(strings.ReplaceAll(decoded, "+", " "))
		}
		current = decoded
	}
	return candidates
}

func webDAVNormalizeSegment(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
