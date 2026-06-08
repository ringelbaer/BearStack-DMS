// Datei puffert haeufig benoetigte Zaehler fuer Listen, Navigation und Statusanzeigen.
package server

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"bearstack/internal/document"
)

const documentCountCacheLimit = 256
const folderValueCacheLimit = 128

type documentCountCacheKey struct {
	query        string
	tags         string
	customFields string
	from         string
	to           string
	trash        bool
}

type documentCountCache struct {
	mu           sync.RWMutex
	counts       map[documentCountCacheKey]int
	folderValues map[documentCountCacheKey][]document.CustomFieldValueFolder
	generation   uint64
}

func (s *Server) countDocuments(ctx context.Context, filter document.ListFilter) (int, error) {
	key := documentCountKey(filter)
	cache := &s.apps.documents.counts

	cache.mu.RLock()
	generation := cache.generation
	if cache.counts != nil {
		if total, ok := cache.counts[key]; ok {
			cache.mu.RUnlock()
			return total, nil
		}
	}
	cache.mu.RUnlock()

	total, err := s.repo.CountDocuments(ctx, filter)
	if err != nil {
		return 0, err
	}

	cache.mu.Lock()
	if generation == cache.generation {
		if cache.counts == nil {
			cache.counts = make(map[documentCountCacheKey]int)
		}
		if len(cache.counts) >= documentCountCacheLimit {
			clear(cache.counts)
		}
		cache.counts[key] = total
	}
	cache.mu.Unlock()

	return total, nil
}

func (s *Server) countDocumentFilters(ctx context.Context, filters []document.ListFilter) ([]int, error) {
	if len(filters) == 0 {
		return nil, nil
	}
	counts := make([]int, len(filters))
	type pendingFilter struct {
		key       documentCountCacheKey
		filter    document.ListFilter
		positions []int
	}
	pendingByKey := map[documentCountCacheKey]int{}
	var pending []pendingFilter
	cache := &s.apps.documents.counts

	cache.mu.RLock()
	generation := cache.generation
	for i, filter := range filters {
		key := documentCountKey(filter)
		if cache.counts != nil {
			if total, ok := cache.counts[key]; ok {
				counts[i] = total
				continue
			}
		}
		if pendingIndex, ok := pendingByKey[key]; ok {
			pending[pendingIndex].positions = append(pending[pendingIndex].positions, i)
			continue
		}
		pendingByKey[key] = len(pending)
		pending = append(pending, pendingFilter{
			key:       key,
			filter:    filter,
			positions: []int{i},
		})
	}
	cache.mu.RUnlock()

	if len(pending) == 0 {
		return counts, nil
	}
	pendingFilters := make([]document.ListFilter, len(pending))
	for i, item := range pending {
		pendingFilters[i] = item.filter
	}
	pendingCounts, err := s.repo.CountDocumentFilters(ctx, pendingFilters)
	if err != nil {
		return nil, err
	}
	for i, total := range pendingCounts {
		for _, position := range pending[i].positions {
			counts[position] = total
		}
	}

	cache.mu.Lock()
	if generation == cache.generation {
		if cache.counts == nil {
			cache.counts = make(map[documentCountCacheKey]int)
		}
		if len(cache.counts)+len(pending) >= documentCountCacheLimit {
			clear(cache.counts)
		}
		for i, item := range pending {
			cache.counts[item.key] = pendingCounts[i]
		}
	}
	cache.mu.Unlock()

	return counts, nil
}

func (s *Server) listFolderCustomFieldValues(ctx context.Context, filter document.ListFilter) ([]document.CustomFieldValueFolder, error) {
	key := documentCountKey(filter)
	cache := &s.apps.documents.counts

	cache.mu.RLock()
	generation := cache.generation
	if cache.folderValues != nil {
		if values, ok := cache.folderValues[key]; ok {
			cache.mu.RUnlock()
			return append([]document.CustomFieldValueFolder(nil), values...), nil
		}
	}
	cache.mu.RUnlock()

	values, err := s.repo.ListFolderCustomFieldValues(ctx, filter)
	if err != nil {
		return nil, err
	}

	cache.mu.Lock()
	if generation == cache.generation {
		if cache.folderValues == nil {
			cache.folderValues = make(map[documentCountCacheKey][]document.CustomFieldValueFolder)
		}
		if len(cache.folderValues) >= folderValueCacheLimit {
			clear(cache.folderValues)
		}
		cache.folderValues[key] = append([]document.CustomFieldValueFolder(nil), values...)
	}
	cache.mu.Unlock()

	return values, nil
}

func (s *Server) invalidateDocumentCountCache() {
	cache := &s.apps.documents.counts
	cache.mu.Lock()
	cache.generation++
	if len(cache.counts) > 0 {
		clear(cache.counts)
	}
	if len(cache.folderValues) > 0 {
		clear(cache.folderValues)
	}
	cache.mu.Unlock()
	s.invalidateDocumentStatisticsCache()
}

func (c *documentCountCache) countSize() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.counts)
}

func documentCountKey(filter document.ListFilter) documentCountCacheKey {
	tags := normalizeTagValues(filter.Tags, "")
	sort.Strings(tags)

	return documentCountCacheKey{
		query:        strings.ToLower(strings.TrimSpace(filter.Query)),
		tags:         strings.Join(tags, "\x00"),
		customFields: customFieldFilterKey(filter.CustomFields),
		from:         dateKey(filter.From),
		to:           dateKey(filter.To),
		trash:        filter.Trash,
	}
}

func customFieldFilterKey(filters []document.CustomFieldFilter) string {
	if len(filters) == 0 {
		return ""
	}
	items := make([]string, 0, len(filters))
	for _, filter := range filters {
		value := document.CleanCustomFieldFilterValue(filter.Value)
		if filter.FieldID <= 0 || value == "" {
			continue
		}
		matchMode := "like"
		if filter.Exact {
			matchMode = "exact"
		}
		items = append(items, strconv.FormatInt(filter.FieldID, 10)+"="+matchMode+"="+strings.ToLower(value))
	}
	sort.Strings(items)
	return strings.Join(items, "\x00")
}

func dateKey(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("2006-01-02")
}
