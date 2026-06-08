package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bearstack/internal/photos"
)

const maxPhotoTraceServerTimingSteps = 40

type photoTraceLogStep struct {
	Name       string            `json:"name"`
	DurationMS float64           `json:"duration_ms"`
	Fields     map[string]string `json:"fields,omitempty"`
}

func (s *Server) withPhotoListTrace(r *http.Request) (*http.Request, *photos.ListTrace) {
	if !photoListTraceRequested(r) {
		return r, nil
	}
	trace := photos.NewListTrace()
	return r.WithContext(photos.ContextWithListTrace(r.Context(), trace)), trace
}

func photoListTraceRequested(r *http.Request) bool {
	if r == nil {
		return false
	}
	if truthy(r.URL.Query().Get("trace")) {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(r.Header.Get("X-BearStack-Photo-Trace"))) {
	case "1", "true", "yes", "on", "photos", "listing":
		return true
	default:
		return false
	}
}

func photoTraceQueryValue(values map[string][]string) string {
	for _, value := range values["trace"] {
		if truthy(value) {
			return value
		}
	}
	return ""
}

func appendPhotoTraceParam(urlValue string, traceValue string) string {
	if strings.TrimSpace(traceValue) == "" {
		return urlValue
	}
	separator := "?"
	if strings.Contains(urlValue, "?") {
		separator = "&"
	}
	return urlValue + separator + "trace=" + url.QueryEscape(traceValue)
}

func annotatePhotoTraceFolderURLs(view *PhotoListingView, traceValue string) {
	if view == nil || strings.TrimSpace(traceValue) == "" {
		return
	}
	for i := range view.Folders {
		view.Folders[i].URL = appendPhotoTraceParam(view.Folders[i].URL, traceValue)
	}
}

func photoFolderPreviewViewCount(folders []PhotoFolderView) int {
	count := 0
	for _, folder := range folders {
		count += len(folder.Previews)
	}
	return count
}

func (s *Server) logPhotoListTrace(r *http.Request, trace *photos.ListTrace) {
	if s == nil || s.log == nil || trace == nil {
		return
	}
	snapshot := trace.Snapshot()
	s.log.Info("photo listing trace",
		"method", r.Method,
		"path", r.URL.Path,
		"query", r.URL.RawQuery,
		"duration", snapshot.TotalDuration.String(),
		"steps", photoTraceLogSteps(snapshot.Steps),
	)
}

func photoTraceLogSteps(steps []photos.ListTraceStep) []photoTraceLogStep {
	out := make([]photoTraceLogStep, 0, len(steps))
	for _, step := range steps {
		fields := map[string]string{}
		for _, field := range step.Fields {
			fields[field.Key] = field.Value
		}
		if len(fields) == 0 {
			fields = nil
		}
		out = append(out, photoTraceLogStep{
			Name:       step.Name,
			DurationMS: durationMilliseconds(step.Duration),
			Fields:     fields,
		})
	}
	return out
}

func photoListServerTimingHeader(snapshot photos.ListTraceSnapshot) string {
	if !snapshot.Enabled {
		return ""
	}
	metrics := []string{
		serverTimingMetric("photo_total", "photos.total", snapshot.TotalDuration),
	}
	limit := len(snapshot.Steps)
	if limit > maxPhotoTraceServerTimingSteps {
		limit = maxPhotoTraceServerTimingSteps
	}
	for i := 0; i < limit; i++ {
		metrics = append(metrics, serverTimingMetric(fmt.Sprintf("photo_%02d", i+1), snapshot.Steps[i].Name, snapshot.Steps[i].Duration))
	}
	return strings.Join(metrics, ", ")
}

func serverTimingMetric(name, description string, duration time.Duration) string {
	return fmt.Sprintf("%s;dur=%.3f;desc=%s", name, durationMilliseconds(duration), strconv.Quote(description))
}

func durationMilliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}
