// Datei rendert die dokumentbasierte Tag-Wortwolke.
package server

import (
	"errors"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"net/url"
	"unicode/utf8"

	"bearstack/internal/document"
)

const (
	tagCloudItemLimit           = 200
	tagCloudRelatedLimit        = 18
	tagCloudCentralLayoutWidth  = 920
	tagCloudCentralLayoutHeight = 380
	tagCloudClusterLayoutWidth  = 460
	tagCloudClusterLayoutHeight = 260
	tagCloudCentralMinSizeRem   = 0.62
	tagCloudCentralMaxSizeRem   = 2.55
	tagCloudRelatedMinSizeRem   = 0.62
	tagCloudRelatedMaxSizeRem   = 1.90
	tagCloudPrimarySizeRem      = 2.65
	tagCloudPrimaryFontWeight   = 860
)

func (s *Server) handleCloud(w http.ResponseWriter, r *http.Request) {
	enabled, err := s.documentCloudEnabled(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	if !enabled {
		s.renderError(w, r, http.StatusNotFound, errors.New("Wolke ist nicht aktiv."))
		return
	}
	cloud, err := s.repo.TagCloud(r.Context(), tagCloudItemLimit, tagCloudRelatedLimit)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	tags, err := s.repo.ListTags(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	s.render(w, r, "cloud.html", PageData{
		Title:    "Wolke",
		Active:   "cloud",
		TagCloud: tagCloudViewFrom(cloud, tags),
		Tags:     tags,
		Notice:   r.URL.Query().Get("notice"),
	})
}

func tagCloudViewFrom(cloud document.TagCloud, tags []document.Tag) TagCloudView {
	styles := tagStyleMap(tags)
	view := TagCloudView{HasPrimaryTags: cloud.HasPrimaryTags}
	if cloud.HasPrimaryTags {
		view.Clusters = make([]TagCloudClusterView, len(cloud.Clusters))
		for i, cluster := range cloud.Clusters {
			primarySizing := tagCloudSizing{SizeRem: tagCloudPrimarySizeRem, Weight: tagCloudPrimaryFontWeight}
			view.Clusters[i] = TagCloudClusterView{
				Primary: tagCloudItemView(cluster.Primary, styles, primarySizing),
				Items: tagCloudItemViewsWithScale(
					cluster.Items,
					cluster.MaxCount,
					styles,
					tagCloudRelatedMinSizeRem,
					tagCloudRelatedMaxSizeRem,
					cluster.Primary.Tag,
				),
			}
			layout := append([]TagCloudItemView{view.Clusters[i].Primary}, view.Clusters[i].Items...)
			tagCloudLayoutItems(layout, tagCloudClusterLayoutWidth, tagCloudClusterLayoutHeight)
			if len(layout) > 0 {
				view.Clusters[i].Primary = layout[0]
				view.Clusters[i].Items = layout[1:]
			}
		}
		view.Empty = len(view.Clusters) == 0
		return view
	}
	view.Items = tagCloudItemViewsWithScale(cloud.Items, cloud.MaxCount, styles, tagCloudCentralMinSizeRem, tagCloudCentralMaxSizeRem)
	tagCloudLayoutItems(view.Items, tagCloudCentralLayoutWidth, tagCloudCentralLayoutHeight)
	view.Empty = len(view.Items) == 0
	return view
}

func tagCloudItemViewsWithScale(items []document.TagCloudItem, maximum int, styles map[string]template.CSS, minSize, maxSize float64, contextTags ...string) []TagCloudItemView {
	views := make([]TagCloudItemView, len(items))
	for i, item := range items {
		views[i] = tagCloudItemView(item, styles, tagCloudScaledSizing(item.Count, maximum, minSize, maxSize), contextTags...)
	}
	return views
}

func tagCloudItemView(item document.TagCloudItem, styles map[string]template.CSS, sizing tagCloudSizing, contextTags ...string) TagCloudItemView {
	tags := append([]string(nil), contextTags...)
	tags = append(tags, item.Tag)
	style, ok := styles[item.Tag]
	if !ok {
		style = tagStyle("#176b87")
	}
	return TagCloudItemView{
		Name:       item.Tag,
		URL:        tagCloudDocumentURL(tags...),
		Count:      item.Count,
		Primary:    item.Primary,
		SizeRem:    sizing.SizeRem,
		CloudStyle: tagCloudBaseStyle(style, sizing),
	}
}

func tagCloudDocumentURL(tags ...string) string {
	q := url.Values{}
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		q.Add("tags", tag)
	}
	if encoded := q.Encode(); encoded != "" {
		return "/documents?" + encoded
	}
	return "/documents"
}

type tagCloudSizing struct {
	SizeRem float64
	Weight  int
}

func (s tagCloudSizing) CSS() template.CSS {
	return template.CSS(fmt.Sprintf("--cloud-size: %.2frem; --cloud-weight: %d;", s.SizeRem, s.Weight))
}

func tagCloudBaseStyle(style template.CSS, sizing tagCloudSizing) template.CSS {
	return template.CSS(fmt.Sprintf("%s %s", style, sizing.CSS()))
}

func tagCloudScaledSizing(count, maximum int, minSize, maxSize float64) tagCloudSizing {
	if maximum < 1 {
		maximum = 1
	}
	if count < 0 {
		count = 0
	}
	ratio := math.Pow(float64(count)/float64(maximum), 0.72)
	size := minSize + (maxSize-minSize)*ratio
	weight := 560 + int(260*ratio)
	return tagCloudSizing{SizeRem: size, Weight: weight}
}

type tagCloudBox struct {
	Left   float64
	Top    float64
	Right  float64
	Bottom float64
}

func tagCloudLayoutItems(items []TagCloudItemView, width, height int) {
	if len(items) == 0 {
		return
	}
	boxes := make([]tagCloudBox, 0, len(items))
	for i := range items {
		box := tagCloudEstimatedBox(items[i])
		x, y := tagCloudPlaceBox(box, boxes, float64(width), float64(height), i)
		box.Left = x - box.Right/2
		box.Top = y - box.Bottom/2
		box.Right = x + box.Right/2
		box.Bottom = y + box.Bottom/2
		boxes = append(boxes, box)
		items[i].CloudStyle = tagCloudItemStyle(items[i].CloudStyle, tagCloudPositionStyle(x, y, float64(width), float64(height)))
	}
}

func tagCloudEstimatedBox(item TagCloudItemView) tagCloudBox {
	size := item.SizeRem
	if size <= 0 {
		size = 1
	}
	runes := utf8.RuneCountInString(item.Name)
	if runes < 1 {
		runes = 1
	}
	width := (float64(runes)*0.57 + 0.76) * size * 16
	height := size * 20
	if item.Primary {
		width *= 1.04
		height *= 1.05
		width += 26
		height += 12
	}
	return tagCloudBox{Right: width, Bottom: height}
}

func tagCloudPlaceBox(box tagCloudBox, boxes []tagCloudBox, width, height float64, index int) (float64, float64) {
	centerX := width / 2
	centerY := height / 2
	if index == 0 {
		return centerX, centerY
	}
	for step := 0; step < 1200; step++ {
		angle := float64(step) * 0.38
		radius := 5 + float64(step)*1.65
		x := centerX + math.Cos(angle)*radius
		y := centerY + math.Sin(angle)*radius*0.56
		if tagCloudFits(box, boxes, x, y, width, height) {
			return x, y
		}
	}
	return tagCloudFallbackPosition(box, boxes, width, height, index)
}

func tagCloudFits(box tagCloudBox, boxes []tagCloudBox, x, y, width, height float64) bool {
	candidate := tagCloudBox{
		Left:   x - box.Right/2,
		Top:    y - box.Bottom/2,
		Right:  x + box.Right/2,
		Bottom: y + box.Bottom/2,
	}
	padding := 3.0
	if candidate.Left < padding || candidate.Top < padding || candidate.Right > width-padding || candidate.Bottom > height-padding {
		return false
	}
	for _, existing := range boxes {
		if candidate.Left < existing.Right+padding &&
			candidate.Right > existing.Left-padding &&
			candidate.Top < existing.Bottom+padding &&
			candidate.Bottom > existing.Top-padding {
			return false
		}
	}
	return true
}

func tagCloudFallbackPosition(box tagCloudBox, boxes []tagCloudBox, width, height float64, index int) (float64, float64) {
	columns := 5
	if width < 520 {
		columns = 3
	}
	for row := 0; row < 16; row++ {
		for col := 0; col < columns; col++ {
			x := (float64(col) + 0.5) * width / float64(columns)
			y := 34 + float64(row)*32 + math.Mod(float64(index), 2)*9
			if tagCloudFits(box, boxes, x, y, width, height) {
				return x, y
			}
		}
	}
	return width/2 + math.Sin(float64(index))*width*0.18, height/2 + math.Cos(float64(index))*height*0.18
}

func tagCloudPositionStyle(x, y, width, height float64) template.CSS {
	left := (x * 100) / width
	top := (y * 100) / height
	return template.CSS(fmt.Sprintf("--cloud-left: %.2f%%; --cloud-top: %.2f%%;", left, top))
}

func tagCloudItemStyle(base, position template.CSS) template.CSS {
	return template.CSS(fmt.Sprintf("%s %s", base, position))
}
