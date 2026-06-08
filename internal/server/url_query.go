// Datei baut und veraendert URL-Query-Parameter fuer Navigation, Filter und UI-Aktionen.
package server

import (
	"net/http"
	"net/url"
)

func cloneQueryValues(values url.Values) url.Values {
	out := url.Values{}
	for key, value := range values {
		out[key] = append([]string(nil), value...)
	}
	return out
}

func clearQueryKeys(values url.Values, keys ...string) url.Values {
	out := cloneQueryValues(values)
	for _, key := range keys {
		out.Del(key)
	}
	return out
}

func pathWithQuery(path string, values url.Values) string {
	if encoded := values.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

func requestURLWithoutQueryKeys(r *http.Request, keys ...string) string {
	return pathWithQuery(r.URL.Path, clearQueryKeys(r.URL.Query(), keys...))
}
