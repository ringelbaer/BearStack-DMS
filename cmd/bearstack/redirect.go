package main

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

func httpToHTTPSRedirectHandler(tlsAddr string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, httpsRedirectURL(r, tlsAddr), http.StatusPermanentRedirect)
	})
}

func httpsRedirectURL(r *http.Request, tlsAddr string) string {
	tlsHost, tlsPort, _ := splitHostPort(tlsAddr)
	reqHost, _, _ := splitHostPort(r.Host)

	host := reqHost
	if host == "" {
		host = tlsHost
	}
	if host == "" {
		host = "localhost"
	}

	targetHost := host
	if tlsPort != "" && tlsPort != "443" {
		targetHost = net.JoinHostPort(host, tlsPort)
	}

	target := &url.URL{
		Scheme: "https",
		Host:   targetHost,
		Path:   "/",
	}
	if r.URL != nil {
		if r.URL.Path != "" {
			target.Path = r.URL.Path
		}
		target.RawPath = r.URL.RawPath
		target.RawQuery = r.URL.RawQuery
	}
	return target.String()
}

func splitHostPort(value string) (string, string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false
	}
	host, port, err := net.SplitHostPort(value)
	if err == nil {
		return strings.Trim(host, "[]"), strings.TrimSpace(port), true
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		return strings.Trim(value, "[]"), "", false
	}
	if strings.Count(value, ":") == 0 {
		return value, "", false
	}
	return strings.Trim(value, "[]"), "", false
}
