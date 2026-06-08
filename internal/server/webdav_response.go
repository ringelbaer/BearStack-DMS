// Datei baut WebDAV-XML- und Metadatenantworten fuer aufgeloeste Ressourcen.
package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"bearstack/internal/document"
)

type davMultistatus struct {
	XMLName   xml.Name      `xml:"DAV: multistatus"`
	Responses []davResponse `xml:"response"`
}

type davResponse struct {
	Href     string      `xml:"href"`
	Propstat davPropstat `xml:"propstat"`
}

type davPropstat struct {
	Prop   davProp `xml:"prop"`
	Status string  `xml:"status"`
}

type davProp struct {
	DisplayName      string          `xml:"displayname,omitempty"`
	ResourceType     davResourceType `xml:"resourcetype"`
	GetContentLength string          `xml:"getcontentlength,omitempty"`
	GetContentType   string          `xml:"getcontenttype,omitempty"`
	GetLastModified  string          `xml:"getlastmodified,omitempty"`
	GetETag          string          `xml:"getetag,omitempty"`
}

type davResourceType struct {
	Collection *struct{} `xml:"collection,omitempty"`
}

func writeWebDAVMultiStatus(w http.ResponseWriter, webDAVPrefix string, resources []webDAVResource) error {
	responses := make([]davResponse, len(resources))
	for i, res := range resources {
		responses[i] = webDAVResponseFor(webDAVPrefix, res)
	}
	payload, err := xml.MarshalIndent(davMultistatus{Responses: responses}, "", "  ")
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", `application/xml; charset="utf-8"`)
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = w.Write(append([]byte(xml.Header), payload...))
	return nil
}

func webDAVResponseFor(webDAVPrefix string, resource webDAVResource) davResponse {
	prop := davProp{
		DisplayName: resource.Name,
	}
	if resource.IsDir {
		prop.ResourceType.Collection = &struct{}{}
	} else {
		prop.GetContentLength = fmt.Sprintf("%d", resource.Document.SizeBytes)
		prop.GetContentType = webDAVDocumentContentType(resource.Document)
		prop.GetLastModified = webDAVModifiedTime(resource.Document, time.Time{}).UTC().Format(http.TimeFormat)
		prop.GetETag = webDAVETag(resource.Document)
	}
	return davResponse{
		Href: webDAVHref(webDAVPrefix, resource),
		Propstat: davPropstat{
			Prop:   prop,
			Status: "HTTP/1.1 200 OK",
		},
	}
}

func webDAVHref(webDAVPrefix string, resource webDAVResource) string {
	if len(resource.Segments) == 0 {
		return webDAVPrefix + "/"
	}
	parts := make([]string, len(resource.Segments))
	for i, segment := range resource.Segments {
		parts[i] = url.PathEscape(segment)
	}
	href := webDAVPrefix + "/" + strings.Join(parts, "/")
	if resource.IsDir {
		href += "/"
	}
	return href
}

func webDAVDocumentContentType(doc document.Document) string {
	if doc.MIMEType != "" {
		return doc.MIMEType
	}
	return mime.TypeByExtension(filepath.Ext(doc.OriginalName))
}

func webDAVModifiedTime(doc document.Document, fallback time.Time) time.Time {
	if !doc.UpdatedAt.IsZero() {
		return doc.UpdatedAt
	}
	if !fallback.IsZero() {
		return fallback
	}
	return doc.UploadedAt
}

func webDAVETag(doc document.Document) string {
	key := fmt.Sprintf("%d:%d:%s:%s", doc.ID, doc.SizeBytes, strings.ToLower(strings.TrimSpace(doc.SHA256)), webDAVModifiedTime(doc, time.Time{}).UTC().Format(time.RFC3339Nano))
	sum := sha256.Sum256([]byte(key))
	return `"` + hex.EncodeToString(sum[:]) + `"`
}
