// Datei liest EXIF- und XMP-Metadaten aus Mediendateien und uebertraegt sie in Foto-Modelle.
package photos

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

func readMetadata(path string) (Metadata, error) {
	meta := Metadata{}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".jpg" || ext == ".jpeg" || ext == ".jpe" {
		if parsed, err := readJPEGMetadata(path); err == nil {
			mergeMetadata(&meta, parsed)
		}
	} else {
		if width, height := imageSize(path); width > 0 && height > 0 {
			meta.Width = width
			meta.Height = height
		}
	}
	for _, sidecarPath := range xmpSidecarPaths(path) {
		sidecar, err := os.ReadFile(sidecarPath)
		if err != nil {
			continue
		}
		parsed := parseXMPMetadataWithBase(string(sidecar), meta)
		if len(parsed.Faces) > 0 {
			meta.Faces = nil
		}
		mergeMetadata(&meta, parsed)
	}
	return meta, nil
}

func parseXMPMetadataWithBase(raw string, base Metadata) Metadata {
	meta := Metadata{
		Faces: parseXMPFacesWithBase(raw, base),
	}
	if rating := parseXMPRating(raw); rating != nil {
		meta.Rating = rating
	}
	return meta
}

func xmpSidecarPaths(path string) []string {
	withoutExt := strings.TrimSuffix(path, filepath.Ext(path))
	candidates := []string{
		path + ".xmp",
		path + ".XMP",
		withoutExt + ".xmp",
		withoutExt + ".XMP",
	}
	paths := make([]string, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		paths = append(paths, candidate)
	}
	return paths
}

func xmpSidecarFingerprint(path string) string {
	var b strings.Builder
	for i, candidate := range xmpSidecarPaths(path) {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		fmt.Fprintf(&b, "%d:%d:%d;", i, info.Size(), info.ModTime().UnixNano())
	}
	return b.String()
}

func parseXMPRating(raw string) *float64 {
	for _, key := range []string{"xmp:Rating", "xap:Rating"} {
		if rating := normalizeXMPRating(xmlAttrValue(raw, key)); rating != nil {
			return rating
		}
		if rating := normalizeXMPRating(xmlTagValue(raw, key)); rating != nil {
			return rating
		}
	}
	return nil
}

func normalizeXMPRating(value string) *float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	rating, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil
	}
	if rating < -1 {
		rating = -1
	}
	if rating > 5 {
		rating = 5
	}
	return &rating
}

func mergeMetadata(dst *Metadata, src Metadata) {
	if src.CapturedAt != nil {
		dst.CapturedAt = src.CapturedAt
	}
	if src.Width > 0 {
		dst.Width = src.Width
	}
	if src.Height > 0 {
		dst.Height = src.Height
	}
	if src.Orientation > 0 {
		dst.Orientation = src.Orientation
	}
	if src.Camera != "" {
		dst.Camera = src.Camera
	}
	if src.Lens != "" {
		dst.Lens = src.Lens
	}
	if src.Rating != nil {
		dst.Rating = src.Rating
	}
	if src.Latitude != nil {
		dst.Latitude = src.Latitude
	}
	if src.Longitude != nil {
		dst.Longitude = src.Longitude
	}
	if len(src.Keywords) > 0 {
		dst.Keywords = src.Keywords
	}
	if len(src.Faces) > 0 {
		dst.Faces = append(dst.Faces, src.Faces...)
	}
}

func readJPEGMetadata(path string) (Metadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return Metadata{}, err
	}
	defer file.Close()

	header := make([]byte, 2)
	if _, err := io.ReadFull(file, header); err != nil {
		return Metadata{}, err
	}
	if header[0] != 0xff || header[1] != 0xd8 {
		return Metadata{}, errors.New("not a jpeg")
	}

	meta := Metadata{}
	parsedEXIF := false
	xmpPayloads := make([]string, 0, 1)
	finish := func() (Metadata, error) {
		for _, raw := range xmpPayloads {
			mergeMetadata(&meta, parseXMPMetadataWithBase(raw, meta))
		}
		return meta, nil
	}
	for {
		markerPrefix := make([]byte, 1)
		if _, err := io.ReadFull(file, markerPrefix); err != nil {
			return finish()
		}
		if markerPrefix[0] != 0xff {
			continue
		}
		marker := make([]byte, 1)
		if _, err := io.ReadFull(file, marker); err != nil {
			return finish()
		}
		for marker[0] == 0xff {
			if _, err := io.ReadFull(file, marker); err != nil {
				return finish()
			}
		}
		if marker[0] == 0xda || marker[0] == 0xd9 {
			return finish()
		}
		if marker[0] == 0x01 || (marker[0] >= 0xd0 && marker[0] <= 0xd7) {
			continue
		}
		var lengthBytes [2]byte
		if _, err := io.ReadFull(file, lengthBytes[:]); err != nil {
			return finish()
		}
		length := int(binary.BigEndian.Uint16(lengthBytes[:]))
		if length < 2 {
			return finish()
		}
		payload := make([]byte, length-2)
		if _, err := io.ReadFull(file, payload); err != nil {
			return finish()
		}
		if width, height := jpegSOFDimensions(marker[0], payload); width > 0 && height > 0 && meta.Width == 0 && meta.Height == 0 {
			meta.Width = width
			meta.Height = height
		}
		if marker[0] == 0xe1 {
			switch {
			case bytes.HasPrefix(payload, []byte("Exif\x00\x00")) && !parsedEXIF:
				if parsed, err := parseEXIF(payload[6:]); err == nil {
					mergeMetadata(&meta, parsed)
					parsedEXIF = true
				}
			case bytes.HasPrefix(payload, []byte("http://ns.adobe.com/xap/1.0/\x00")):
				xmpPayloads = append(xmpPayloads, string(payload[len("http://ns.adobe.com/xap/1.0/\x00"):]))
			}
		}
	}
}

func jpegSOFDimensions(marker byte, payload []byte) (int, int) {
	switch marker {
	case 0xc0, 0xc1, 0xc2, 0xc3, 0xc5, 0xc6, 0xc7, 0xc9, 0xca, 0xcb, 0xcd, 0xce, 0xcf:
	default:
		return 0, 0
	}
	if len(payload) < 5 {
		return 0, 0
	}
	height := int(binary.BigEndian.Uint16(payload[1:3]))
	width := int(binary.BigEndian.Uint16(payload[3:5]))
	return width, height
}
func parseEXIF(data []byte) (Metadata, error) {
	if len(data) < 8 {
		return Metadata{}, errors.New("short exif")
	}
	var order binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return Metadata{}, errors.New("unknown byte order")
	}
	if order.Uint16(data[2:4]) != 42 {
		return Metadata{}, errors.New("invalid tiff")
	}
	ifd0 := int(order.Uint32(data[4:8]))
	meta := Metadata{}
	values := readIFD(data, order, ifd0)
	makeName := valueString(values[0x010f])
	model := valueString(values[0x0110])
	meta.Camera = strings.TrimSpace(strings.TrimSpace(makeName + " " + model))
	meta.Orientation = int(valueUint(values[0x0112]))
	meta.Keywords = splitKeywords(valueString(values[0x9c9e]))
	if dt := parseExifTime(valueString(values[0x0132])); dt != nil {
		meta.CapturedAt = dt
	}
	if exifOffset := int(valueUint(values[0x8769])); exifOffset > 0 {
		exifValues := readIFD(data, order, exifOffset)
		if dt := parseExifTime(firstString(exifValues[0x9003], exifValues[0x9004])); dt != nil {
			meta.CapturedAt = dt
		}
		if width := int(valueUint(exifValues[0xa002])); width > 0 {
			meta.Width = width
		}
		if height := int(valueUint(exifValues[0xa003])); height > 0 {
			meta.Height = height
		}
		meta.Lens = valueString(exifValues[0xa434])
	}
	if gpsOffset := int(valueUint(values[0x8825])); gpsOffset > 0 {
		gpsValues := readIFD(data, order, gpsOffset)
		meta.Latitude, meta.Longitude = gpsCoordinates(gpsValues)
	}
	return meta, nil
}

type exifValue struct {
	Type  uint16
	Count uint32
	Raw   []byte
	Order binary.ByteOrder
}

func readIFD(data []byte, order binary.ByteOrder, offset int) map[uint16]exifValue {
	values := map[uint16]exifValue{}
	if offset < 0 || offset+2 > len(data) {
		return values
	}
	count := int(order.Uint16(data[offset : offset+2]))
	pos := offset + 2
	for i := 0; i < count; i++ {
		if pos+12 > len(data) {
			return values
		}
		tag := order.Uint16(data[pos : pos+2])
		valueType := order.Uint16(data[pos+2 : pos+4])
		valueCount := order.Uint32(data[pos+4 : pos+8])
		valueLen := int(valueCount) * exifTypeSize(valueType)
		var raw []byte
		if valueLen <= 4 {
			raw = append([]byte(nil), data[pos+8:pos+8+valueLen]...)
		} else {
			valueOffset := int(order.Uint32(data[pos+8 : pos+12]))
			if valueOffset >= 0 && valueOffset+valueLen <= len(data) {
				raw = append([]byte(nil), data[valueOffset:valueOffset+valueLen]...)
			}
		}
		values[tag] = exifValue{Type: valueType, Count: valueCount, Raw: raw, Order: order}
		pos += 12
	}
	return values
}

func exifTypeSize(valueType uint16) int {
	switch valueType {
	case 1, 2, 7:
		return 1
	case 3:
		return 2
	case 4, 9:
		return 4
	case 5, 10:
		return 8
	default:
		return 0
	}
}

func valueString(value exifValue) string {
	if len(value.Raw) == 0 {
		return ""
	}
	if value.Type == 1 && value.Count > 1 {
		return decodeUTF16LE(value.Raw)
	}
	if value.Type == 2 || value.Type == 7 || value.Type == 1 {
		return strings.TrimRight(string(value.Raw), "\x00 ")
	}
	return ""
}

func firstString(values ...exifValue) string {
	for _, value := range values {
		if s := valueString(value); s != "" {
			return s
		}
	}
	return ""
}

func valueUint(value exifValue) uint32 {
	if len(value.Raw) == 0 {
		return 0
	}
	switch value.Type {
	case 3:
		if len(value.Raw) >= 2 {
			return uint32(value.Order.Uint16(value.Raw[:2]))
		}
	case 4:
		if len(value.Raw) >= 4 {
			return value.Order.Uint32(value.Raw[:4])
		}
	}
	return 0
}

func rationalValues(value exifValue) []float64 {
	if value.Type != 5 || len(value.Raw) < 8 {
		return nil
	}
	values := make([]float64, 0, len(value.Raw)/8)
	for i := 0; i+8 <= len(value.Raw); i += 8 {
		num := value.Order.Uint32(value.Raw[i : i+4])
		den := value.Order.Uint32(value.Raw[i+4 : i+8])
		if den == 0 {
			values = append(values, 0)
			continue
		}
		values = append(values, float64(num)/float64(den))
	}
	return values
}

func gpsCoordinates(values map[uint16]exifValue) (*float64, *float64) {
	lat := gpsDecimal(valueString(values[1]), rationalValues(values[2]))
	lon := gpsDecimal(valueString(values[3]), rationalValues(values[4]))
	return lat, lon
}

func gpsDecimal(ref string, parts []float64) *float64 {
	if len(parts) < 3 {
		return nil
	}
	value := parts[0] + parts[1]/60 + parts[2]/3600
	ref = strings.ToUpper(strings.TrimSpace(ref))
	if ref == "S" || ref == "W" {
		value *= -1
	}
	return &value
}

func parseExifTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	layouts := []string{"2006:01:02 15:04:05", "2006-01-02 15:04:05", time.RFC3339}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return &parsed
		}
	}
	return nil
}

func decodeUTF16LE(raw []byte) string {
	if len(raw)%2 == 1 {
		raw = raw[:len(raw)-1]
	}
	values := make([]uint16, 0, len(raw)/2)
	for i := 0; i+1 < len(raw); i += 2 {
		v := binary.LittleEndian.Uint16(raw[i : i+2])
		if v == 0 {
			break
		}
		values = append(values, v)
	}
	return string(utf16.Decode(values))
}

func splitKeywords(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ';' || r == ',' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		key := strings.ToLower(field)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, field)
	}
	return out
}

func parseXMPFacesWithBase(raw string, base Metadata) []Face {
	root, err := parseXMPDocument(raw)
	if err != nil {
		return nil
	}
	orientation := base.Orientation
	if orientation <= 0 {
		orientation = 1
	}
	if xmpOrientation := xmpOrientation(root); xmpOrientation > 0 {
		orientation = xmpOrientation
	}
	var faces []Face
	for _, item := range xmpDescendants(root, "li") {
		region := xmpFaceRegionNode(item)
		name := xmpLocalValue(region, "Name")
		if name == "" {
			continue
		}
		if !strings.EqualFold(xmpLocalValue(region, "Type"), "Face") {
			continue
		}
		area := xmpFirstChild(region, "Area")
		if area == nil {
			area = region
		}
		x, y, w, h, ok := xmpAreaValues(area)
		if !ok {
			continue
		}
		face, ok := normalizedXMPFace(name, x, y, w, h, orientation)
		if !ok {
			continue
		}
		faces = append(faces, face)
	}
	return faces
}

type xmpNode struct {
	Name     string
	Attrs    map[string]string
	Text     string
	Children []*xmpNode
}

func parseXMPDocument(raw string) (*xmpNode, error) {
	decoder := xml.NewDecoder(strings.NewReader(raw))
	root := &xmpNode{Name: "root"}
	stack := []*xmpNode{root}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch token := token.(type) {
		case xml.StartElement:
			node := &xmpNode{Name: token.Name.Local, Attrs: map[string]string{}}
			for _, attr := range token.Attr {
				node.Attrs[attr.Name.Local] = attr.Value
			}
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, node)
			stack = append(stack, node)
		case xml.CharData:
			stack[len(stack)-1].Text += string(token)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return root, nil
}

func xmpFaceRegionNode(item *xmpNode) *xmpNode {
	if item == nil {
		return nil
	}
	if description := xmpFirstChild(item, "Description"); description != nil {
		return description
	}
	return item
}

func xmpAreaValues(area *xmpNode) (float64, float64, float64, float64, bool) {
	if area == nil {
		return 0, 0, 0, 0, false
	}
	x, okX := xmpFloatValue(area, "x")
	y, okY := xmpFloatValue(area, "y")
	w, okW := xmpFloatValue(area, "w")
	h, okH := xmpFloatValue(area, "h")
	return x, y, w, h, okX && okY && okW && okH
}

func xmpOrientation(root *xmpNode) int {
	if value := xmpLocalAttrRecursive(root, "Orientation"); value != "" {
		if orientation, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			return orientation
		}
	}
	for _, node := range xmpDescendants(root, "Orientation") {
		if orientation, err := strconv.Atoi(strings.TrimSpace(xmpTextContent(node))); err == nil {
			return orientation
		}
	}
	return 0
}

func xmpLocalValue(node *xmpNode, local string) string {
	if node == nil {
		return ""
	}
	if value := xmpLocalAttr(node, local); value != "" {
		return value
	}
	if child := xmpFirstChild(node, local); child != nil {
		return strings.TrimSpace(xmpTextContent(child))
	}
	return ""
}

func xmpFloatValue(node *xmpNode, local string) (float64, bool) {
	value := xmpLocalValue(node, local)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return parsed, err == nil
}

func xmpLocalAttr(node *xmpNode, local string) string {
	if node == nil {
		return ""
	}
	for name, value := range node.Attrs {
		if strings.EqualFold(name, local) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func xmpLocalAttrRecursive(node *xmpNode, local string) string {
	if value := xmpLocalAttr(node, local); value != "" {
		return value
	}
	for _, child := range node.Children {
		if value := xmpLocalAttrRecursive(child, local); value != "" {
			return value
		}
	}
	return ""
}

func xmpFirstChild(node *xmpNode, local string) *xmpNode {
	if node == nil {
		return nil
	}
	for _, child := range node.Children {
		if strings.EqualFold(child.Name, local) {
			return child
		}
	}
	return nil
}

func xmpDescendants(node *xmpNode, local string) []*xmpNode {
	if node == nil {
		return nil
	}
	var matches []*xmpNode
	for _, child := range node.Children {
		if strings.EqualFold(child.Name, local) {
			matches = append(matches, child)
		}
		matches = append(matches, xmpDescendants(child, local)...)
	}
	return matches
}

func xmpTextContent(node *xmpNode) string {
	if node == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(node.Text)
	for _, child := range node.Children {
		b.WriteString(xmpTextContent(child))
	}
	return b.String()
}

func normalizedXMPFace(name string, x, y, w, h float64, orientation int) (Face, bool) {
	name = strings.TrimSpace(name)
	if name == "" || w <= 0 || h <= 0 {
		return Face{}, false
	}
	if orientation > 4 {
		x, y = y, x
		w, h = h, w
	}
	var swapX, swapY float64
	switch orientation {
	case 2, 6:
		swapX = 1
	case 3, 7:
		swapX = 1
		swapY = 1
	case 4, 8:
		swapY = 1
	}
	width := clampUnit(w)
	height := clampUnit(h)
	left := clampUnit(math.Abs(x-swapX) - width/2)
	top := clampUnit(math.Abs(y-swapY) - height/2)
	if left+width > 1 {
		width = 1 - left
	}
	if top+height > 1 {
		height = 1 - top
	}
	if width <= 0 || height <= 0 {
		return Face{}, false
	}
	return Face{Name: name, X: left, Y: top, Width: width, Height: height}, true
}

func clampUnit(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func xmlTagValue(raw, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	start := strings.Index(raw, open)
	if start < 0 {
		return ""
	}
	start += len(open)
	end := strings.Index(raw[start:], close)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(raw[start : start+end])
}

func xmlAttrValue(raw, attr string) string {
	key := attr + `="`
	start := strings.Index(raw, key)
	if start < 0 {
		return ""
	}
	start += len(key)
	end := strings.Index(raw[start:], `"`)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(raw[start : start+end])
}
