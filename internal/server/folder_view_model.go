// Datei baut View-Modelle fuer Ordnernavigation, Dokumentlisten und zugehoerige UI-Zustaende.
package server

import (
	"sort"
	"strings"

	"bearstack/internal/document"
)

const (
	folderViewKindTag                 = "tag"
	folderViewKindFieldValue          = "field_value"
	folderViewKindSearchFavoritesRoot = "search_favorites"
	folderViewKindSearchFavorite      = "search_favorite"
)

type folderViewItem struct {
	Tag        document.Tag
	Name       string
	Kind       string
	Count      int
	FieldID    int64
	FieldLabel string
	Value      string
	FavoriteID int64
	Selection  virtualFolderSelection
	Redundant  bool
}

func folderViewItemsFromTags(tags []document.Tag, selection virtualFolderSelection) []folderViewItem {
	items := make([]folderViewItem, len(tags))
	for i, tag := range tags {
		items[i] = folderViewItem{
			Tag:       tag,
			Name:      tag.Name,
			Kind:      folderViewKindTag,
			Count:     tag.Count,
			Selection: selection.AppendTag(tag.Name),
		}
	}
	return items
}

func folderViewItemsFromCustomFieldValues(values []document.CustomFieldValueFolder, selection virtualFolderSelection) []folderViewItem {
	items := make([]folderViewItem, len(values))
	for i, value := range values {
		items[i] = folderViewItem{
			Tag: document.Tag{
				ID:    value.FieldID,
				Name:  value.Value,
				Count: value.Count,
			},
			Name:       value.Value,
			Kind:       folderViewKindFieldValue,
			Count:      value.Count,
			FieldID:    value.FieldID,
			FieldLabel: value.FieldLabel,
			Value:      value.Value,
			Selection:  selection.AppendCustomFieldValue(value),
		}
	}
	return items
}

func searchFavoritesRootViewItem(count int) folderViewItem {
	return folderViewItem{
		Tag: document.Tag{
			Name:  searchFavoritesFolderName,
			Count: count,
		},
		Name:  searchFavoritesFolderName,
		Kind:  folderViewKindSearchFavoritesRoot,
		Count: count,
		Selection: virtualFolderSelection{Segments: []virtualFolderSegment{{
			Kind: virtualFolderSegmentTag,
			Tag:  searchFavoritesFolderName,
		}}},
	}
}

func searchFavoriteViewItem(favorite document.SearchFavorite, count int) folderViewItem {
	return folderViewItem{
		Tag: document.Tag{
			ID:    favorite.ID,
			Name:  favorite.Name,
			Count: count,
		},
		Name:       favorite.Name,
		Kind:       folderViewKindSearchFavorite,
		Count:      count,
		FavoriteID: favorite.ID,
		Selection: virtualFolderSelection{Segments: []virtualFolderSegment{
			{Kind: virtualFolderSegmentTag, Tag: searchFavoritesFolderName},
			{Kind: virtualFolderSegmentTag, Tag: favorite.Name},
		}},
	}
}

func sortFolderViewItems(items []folderViewItem) {
	sort.SliceStable(items, func(i, j int) bool {
		left := strings.ToLower(escapePathComponent(items[i].Name))
		right := strings.ToLower(escapePathComponent(items[j].Name))
		if left == right {
			return strings.ToLower(items[i].FieldLabel) < strings.ToLower(items[j].FieldLabel)
		}
		return left < right
	})
}

func markRedundantFolderViewItems(items []folderViewItem, currentLevelCount int) {
	if currentLevelCount <= 0 {
		return
	}
	for i := range items {
		switch items[i].Kind {
		case folderViewKindSearchFavoritesRoot, folderViewKindSearchFavorite:
			continue
		default:
			items[i].Redundant = items[i].Count == currentLevelCount
		}
	}
}
