// Datei parst Paginierungsparameter und berechnet Seiteninformationen fuer Listenansichten.
package server

import (
	"net/http"
	"strconv"
	"strings"
)

func normalizeDocumentPageSize(value int) int {
	for _, option := range documentPageSizeOptions {
		if value == option {
			return value
		}
	}
	return defaultDocumentPageSize
}

func pageCount(total, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return 1 + (total-1)/pageSize
}

func documentPageSizeFromString(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	for _, option := range documentPageSizeOptions {
		if parsed == option {
			return parsed, true
		}
	}
	return 0, false
}

func documentListPaginationData(r *http.Request, page, perPage, count, total int) PaginationData {
	perPage = normalizeDocumentPageSize(perPage)
	data := paginationData(r, page, (page-1)*perPage+count < total)
	data.Total = total
	data.PerPage = perPage
	data.PerPageOptions = documentPageSizeOptions
	data.PageSizeReturnURL = pageSizeReturnURL(r)
	data.DocumentList = true
	if count > 0 && total > 0 {
		data.Start = (page-1)*perPage + 1
		data.End = data.Start + count - 1
		if data.End > total {
			data.End = total
		}
	}
	return data
}

func paginationData(r *http.Request, page int, hasNext bool) PaginationData {
	data := PaginationData{Page: page}
	if page > 1 {
		data.PrevURL = pageURL(r, page-1)
	}
	if hasNext {
		data.NextURL = pageURL(r, page+1)
	}
	return data
}

func pageSizeReturnURL(r *http.Request) string {
	return requestURLWithoutQueryKeys(r, "notice", "highlight", "page")
}

func pageURL(r *http.Request, page int) string {
	q := clearQueryKeys(r.URL.Query(), "notice", "highlight")
	if page <= 1 {
		q.Del("page")
	} else {
		q.Set("page", strconv.Itoa(page))
	}
	return pathWithQuery(r.URL.Path, q)
}
