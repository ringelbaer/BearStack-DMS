// Datei baut Suchtexte und Suchfilter fuer Medien, Ordner und Blogbeitraege im Fotoindex.
package photos

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"bearstack/internal/searchtext"
)

type queryTerm struct {
	Field   string
	Value   string
	Negated bool
}

type queryExpression struct {
	Groups [][]queryNode
	HasOR  bool
}

type queryNode struct {
	Raw      string
	Term     queryTerm
	Skip     bool
	NOf      int
	NOfTerms []queryTerm
}

func matchesQuery(media Media, query string) bool {
	return compileMediaQuery(query).matches(media)
}

func matchesBlogQuery(post BlogPost, query string) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return true
	}
	text := strings.Join([]string{
		post.Name,
		post.Path,
		post.Text,
		strings.Join(post.Tags, " "),
	}, " ")
	for _, group := range splitORTokens(query) {
		ok := true
		for _, raw := range group {
			if strings.EqualFold(raw, "and") || strings.EqualFold(raw, "or") {
				continue
			}
			term := parseTerm(raw)
			match := false
			switch term.Field {
			case "tag":
				match = matchTag(post.Tags, term.Value)
			case "file_name":
				match = matchText(post.Name, term.Value)
			case "directory":
				match = matchText(parentPath(post.Path), term.Value)
			case "type":
				match = strings.EqualFold(term.Value, MediaTypeBlog)
			default:
				match = matchText(text, term.Value)
			}
			if term.Negated {
				match = !match
			}
			if !match {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func matchesFolderQuery(folder Folder, query string) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return true
	}
	text := strings.Join([]string{
		folder.Name,
		folder.Path,
		strings.Join(folder.Tags, " "),
	}, " ")
	for _, group := range splitORTokens(query) {
		ok := true
		for _, raw := range group {
			if strings.EqualFold(raw, "and") || strings.EqualFold(raw, "or") {
				continue
			}
			term := parseTerm(raw)
			match := false
			switch term.Field {
			case "tag":
				match = matchTag(folder.Tags, term.Value)
			case "file_name":
				match = matchText(folder.Name, term.Value)
			case "directory":
				match = matchText(parentPath(folder.Path), term.Value)
			case "type":
				value := strings.ToLower(strings.TrimSpace(term.Value))
				match = value == "folder" || value == "directory" || value == "album"
			default:
				match = matchText(text, term.Value)
			}
			if term.Negated {
				match = !match
			}
			if !match {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

type compiledMediaQuery struct {
	groups [][]compiledQueryTerm
}

type compiledQueryTerm struct {
	raw      string
	term     queryTerm
	skip     bool
	nOf      int
	nOfTerms []queryTerm
}

func compileMediaQuery(query string) compiledMediaQuery {
	expression := parseQueryExpression(query)
	if len(expression.Groups) == 0 {
		return compiledMediaQuery{}
	}
	groups := make([][]compiledQueryTerm, 0, len(expression.Groups))
	for _, expressionGroup := range expression.Groups {
		group := make([]compiledQueryTerm, 0, len(expressionGroup))
		for _, node := range expressionGroup {
			compiled := compiledQueryTerm{
				raw:      node.Raw,
				term:     node.Term,
				skip:     node.Skip,
				nOf:      node.NOf,
				nOfTerms: node.NOfTerms,
			}
			group = append(group, compiled)
		}
		groups = append(groups, group)
	}
	return compiledMediaQuery{groups: groups}
}

func (query compiledMediaQuery) matches(media Media) bool {
	if len(query.groups) == 0 {
		return true
	}
	for _, group := range query.groups {
		if matchesCompiledQueryGroup(media, group) {
			return true
		}
	}
	return false
}

func matchesCompiledQueryGroup(media Media, group []compiledQueryTerm) bool {
	for _, compiled := range group {
		if compiled.skip {
			continue
		}
		if compiled.nOf > 0 {
			if !matchesCompiledNOf(media, compiled.nOfTerms, compiled.nOf) {
				return false
			}
			continue
		}
		ok := matchesTerm(media, compiled.term)
		if compiled.term.Negated {
			ok = !ok
		}
		if !ok {
			return false
		}
	}
	return true
}

func (query compiledMediaQuery) matchesRow(row cachedMediaRow) bool {
	if len(query.groups) == 0 {
		return true
	}
	matcher := cachedMediaRowMatcher{row: row}
	for _, group := range query.groups {
		if matchesCompiledQueryGroupRow(&matcher, group) {
			return true
		}
	}
	return false
}

type cachedMediaRowMatcher struct {
	row         cachedMediaRow
	baseText    string
	baseSet     bool
	keywords    string
	keywordsSet bool
	tags        string
	tagsSet     bool
	faces       string
	facesSet    bool
}

func matchesCompiledQueryGroupRow(matcher *cachedMediaRowMatcher, group []compiledQueryTerm) bool {
	for _, compiled := range group {
		if compiled.skip {
			continue
		}
		if compiled.nOf > 0 {
			if !matchesCompiledNOfRow(matcher, compiled.nOfTerms, compiled.nOf) {
				return false
			}
			continue
		}
		ok := matcher.matchesTerm(compiled.term)
		if compiled.term.Negated {
			ok = !ok
		}
		if !ok {
			return false
		}
	}
	return true
}

func parseQueryExpression(query string) queryExpression {
	query = strings.TrimSpace(query)
	if query == "" {
		return queryExpression{}
	}
	tokens := tokenizeQuery(query)
	rawGroups := splitORTokensFromTokens(tokens)
	expression := queryExpression{
		Groups: make([][]queryNode, 0, len(rawGroups)),
		HasOR:  queryHasOR(tokens),
	}
	for _, rawGroup := range rawGroups {
		group := make([]queryNode, 0, len(rawGroup))
		for _, raw := range rawGroup {
			group = append(group, parseQueryNode(raw))
		}
		expression.Groups = append(expression.Groups, group)
	}
	return expression
}

func queryHasOR(tokens []string) bool {
	for _, token := range tokens {
		if strings.EqualFold(token, "or") {
			return true
		}
	}
	return false
}

func parseQueryNode(raw string) queryNode {
	node := queryNode{Raw: raw}
	if strings.EqualFold(raw, "and") || strings.EqualFold(raw, "or") {
		node.Skip = true
		return node
	}
	if strings.HasPrefix(strings.ToLower(raw), "2-of:(") && strings.HasSuffix(raw, ")") {
		node.NOf = 2
		node.NOfTerms = parseNOfTerms(raw)
		return node
	}
	node.Term = parseTerm(raw)
	return node
}

func splitORTokens(query string) [][]string {
	return splitORTokensFromTokens(tokenizeQuery(query))
}

func splitORTokensFromTokens(tokens []string) [][]string {
	var groups [][]string
	var current []string
	for _, token := range tokens {
		if strings.EqualFold(token, "or") {
			if len(current) > 0 {
				groups = append(groups, current)
				current = nil
			}
			continue
		}
		current = append(current, token)
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	if len(groups) == 0 && len(tokens) > 0 {
		groups = [][]string{tokens}
	}
	return groups
}

func tokenizeQuery(query string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := false
	quote := rune(0)
	depth := 0
	for _, r := range query {
		switch {
		case inQuote:
			if r == quote {
				inQuote = false
			} else {
				current.WriteRune(r)
			}
		case r == '"' || r == '\'':
			inQuote = true
			quote = r
		case r == '(':
			depth++
			current.WriteRune(r)
		case r == ')':
			if depth > 0 {
				depth--
			}
			current.WriteRune(r)
		case (r == ' ' || r == '\t' || r == '\n') && depth == 0:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

func parseTerm(raw string) queryTerm {
	raw = strings.TrimSpace(raw)
	term := queryTerm{Value: strings.Trim(raw, `"'`)}
	if strings.HasPrefix(term.Value, "-") {
		term.Negated = true
		term.Value = strings.TrimPrefix(term.Value, "-")
	}
	if strings.HasPrefix(term.Value, "!") {
		term.Negated = true
		term.Value = strings.TrimPrefix(term.Value, "!")
	}
	if field, value, ok := strings.Cut(term.Value, "!:"); ok {
		term.Field = normalizeField(field)
		term.Value = strings.Trim(value, `"'()`)
		term.Negated = true
		return term
	}
	if field, value, ok := strings.Cut(term.Value, ":"); ok {
		term.Field = normalizeField(field)
		term.Value = strings.Trim(value, `"'()`)
		return term
	}
	return term
}

func normalizeField(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "any_text", "text", "":
		return ""
	case "file", "filename", "file_name", "name":
		return "file_name"
	case "dir", "directory", "path":
		return "directory"
	case "keyword", "keywords":
		return "keyword"
	case "tag", "tags":
		return "tag"
	case "person", "face", "faces":
		return "person"
	case "camera", "make", "model":
		return "camera"
	case "lens":
		return "lens"
	case "date", "year":
		return "date"
	case "orientation":
		return "orientation"
	case "resolution", "mpx", "megapixel":
		return "resolution"
	case "type", "media":
		return "type"
	case "gps", "map", "position":
		return "gps"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func matchesTerm(media Media, term queryTerm) bool {
	value := strings.TrimSpace(term.Value)
	if value == "" {
		return true
	}
	switch term.Field {
	case "file_name":
		return matchText(media.Name, value)
	case "directory":
		return matchText(media.Directory, value)
	case "keyword":
		return matchText(strings.Join(keywordAndTagText(media), " "), value)
	case "tag":
		return matchTag(media.Tags, value)
	case "person":
		return matchText(faceText(media.Faces), value)
	case "camera":
		return matchText(media.Camera, value)
	case "lens":
		return matchText(media.Lens, value)
	case "date":
		return matchDate(media, value)
	case "orientation":
		return strings.EqualFold(media.Orientation, value)
	case "resolution":
		return matchResolution(media, value)
	case "type":
		return strings.EqualFold(media.Type, normalizeMediaType(value))
	case "gps":
		hasGPS := media.Latitude != nil && media.Longitude != nil
		switch strings.ToLower(value) {
		case "1", "true", "yes", "on":
			return hasGPS
		case "0", "false", "no", "off":
			return !hasGPS
		default:
			return hasGPS
		}
	default:
		return matchText(searchText(media), value)
	}
}

func (matcher *cachedMediaRowMatcher) matchesTerm(term queryTerm) bool {
	value := strings.TrimSpace(term.Value)
	if value == "" {
		return true
	}
	row := matcher.row
	switch term.Field {
	case "file_name":
		return matchText(row.Name, value)
	case "directory":
		return matchText(row.Directory, value)
	case "keyword":
		return matcher.matchKeyword(value)
	case "tag":
		return matchTag(tagsFromJSON(row.Tags), value)
	case "person":
		return matchText(matcher.faceText(), value)
	case "camera":
		return matchText(row.Camera, value)
	case "lens":
		return matchText(row.Lens, value)
	case "date":
		return matchRowDate(row, value)
	case "orientation":
		return strings.EqualFold(row.Orientation, value)
	case "resolution":
		return matchRowResolution(row, value)
	case "type":
		return strings.EqualFold(row.Type, normalizeMediaType(value))
	case "gps":
		hasGPS := row.Latitude.Valid && row.Longitude.Valid
		switch strings.ToLower(value) {
		case "1", "true", "yes", "on":
			return hasGPS
		case "0", "false", "no", "off":
			return !hasGPS
		default:
			return hasGPS
		}
	default:
		return matcher.matchSearchText(value)
	}
}

func keywordAndTagText(media Media) []string {
	values := make([]string, 0, len(media.Keywords)+len(media.Tags))
	values = append(values, media.Keywords...)
	values = append(values, media.Tags...)
	return values
}

func matchDate(media Media, value string) bool {
	date := mediaDate(media)
	if len(value) == 4 {
		year, err := strconv.Atoi(value)
		return err == nil && date.Year() == year
	}
	if len(value) == 7 && value[4] == '-' {
		return date.Format("2006-01") == value
	}
	return date.Format("2006-01-02") == value
}

func matchResolution(media Media, value string) bool {
	return matchResolutionValue(media.Width, media.Height, value)
}

func matchRowDate(row cachedMediaRow, value string) bool {
	date := cachedRowDate(row)
	if len(value) == 4 {
		year, err := strconv.Atoi(value)
		return err == nil && date.Year() == year
	}
	if len(value) == 7 && value[4] == '-' {
		return date.Format("2006-01") == value
	}
	return date.Format("2006-01-02") == value
}

func matchRowResolution(row cachedMediaRow, value string) bool {
	return matchResolutionValue(row.Width, row.Height, value)
}

func matchResolutionValue(width, height int, value string) bool {
	if width <= 0 || height <= 0 {
		return false
	}
	mpx := float64(width*height) / 1_000_000
	value = strings.TrimSpace(value)
	for _, op := range []string{">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(value, op) {
			threshold, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(value, op)), 64)
			if err != nil {
				return false
			}
			switch op {
			case ">=":
				return mpx >= threshold
			case "<=":
				return mpx <= threshold
			case ">":
				return mpx > threshold
			case "<":
				return mpx < threshold
			case "=":
				return mpx == threshold
			}
		}
	}
	threshold, err := strconv.ParseFloat(value, 64)
	return err == nil && mpx >= threshold
}

func parseNOfTerms(raw string) []queryTerm {
	open := strings.Index(raw, "(")
	close := strings.LastIndex(raw, ")")
	if open < 0 || close <= open {
		return nil
	}
	parts := strings.Split(raw[open+1:close], ",")
	terms := make([]queryTerm, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		terms = append(terms, parseTerm(part))
	}
	return terms
}

func matchesCompiledNOf(media Media, terms []queryTerm, n int) bool {
	matches := 0
	for _, term := range terms {
		if matchesTerm(media, term) {
			matches++
		}
	}
	return matches >= n
}

func matchesCompiledNOfRow(matcher *cachedMediaRowMatcher, terms []queryTerm, n int) bool {
	matches := 0
	for _, term := range terms {
		if matcher.matchesTerm(term) {
			matches++
		}
	}
	return matches >= n
}

func (matcher *cachedMediaRowMatcher) matchKeyword(value string) bool {
	var b strings.Builder
	appendSearchPart(&b, matcher.keywordText())
	appendSearchPart(&b, matcher.tagText())
	return matchText(b.String(), value)
}

func (matcher *cachedMediaRowMatcher) matchSearchText(value string) bool {
	base := matcher.searchBaseText()
	if matchText(base, value) {
		return true
	}
	keywords := matcher.keywordText()
	tags := matcher.tagText()
	faces := matcher.faceText()
	if keywords == "" && tags == "" && faces == "" {
		return false
	}
	var b strings.Builder
	appendSearchPart(&b, base)
	appendSearchPart(&b, keywords)
	appendSearchPart(&b, tags)
	appendSearchPart(&b, faces)
	return matchText(b.String(), value)
}

func (matcher *cachedMediaRowMatcher) searchBaseText() string {
	if matcher.baseSet {
		return matcher.baseText
	}
	matcher.baseText = rowBaseSearchText(matcher.row)
	matcher.baseSet = true
	return matcher.baseText
}

func (matcher *cachedMediaRowMatcher) keywordText() string {
	if matcher.keywordsSet {
		return matcher.keywords
	}
	matcher.keywords = stringArrayTextFromJSON(matcher.row.Keywords)
	matcher.keywordsSet = true
	return matcher.keywords
}

func (matcher *cachedMediaRowMatcher) tagText() string {
	if matcher.tagsSet {
		return matcher.tags
	}
	matcher.tags = strings.Join(tagsFromJSON(matcher.row.Tags), " ")
	matcher.tagsSet = true
	return matcher.tags
}

func (matcher *cachedMediaRowMatcher) faceText() string {
	if matcher.facesSet {
		return matcher.faces
	}
	matcher.faces = faceTextFromJSON(matcher.row.Faces)
	matcher.facesSet = true
	return matcher.faces
}

func rowBaseSearchText(row cachedMediaRow) string {
	var b strings.Builder
	b.Grow(len(row.Name) + len(row.Path) + len(row.Directory) + len(row.Camera) + len(row.Lens) + len(row.Orientation) + len(row.Type) + 48)
	appendSearchPart(&b, row.Name)
	appendSearchPart(&b, row.Path)
	appendSearchPart(&b, row.Directory)
	appendSearchPart(&b, row.Camera)
	appendSearchPart(&b, row.Lens)
	appendSearchPart(&b, row.Orientation)
	appendSearchPart(&b, row.Type)
	if date := cachedRowDate(row); !date.IsZero() {
		appendSearchPart(&b, date.Format("2006-01-02"))
		appendSearchPart(&b, strconv.Itoa(date.Year()))
	}
	return b.String()
}

func cachedRowDate(row cachedMediaRow) time.Time {
	if row.CapturedAt != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, row.CapturedAt); err == nil {
			return parsed
		}
	}
	return time.Unix(0, row.ModTimeUnixNano)
}

func stringArrayTextFromJSON(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "[]" || value == "null" {
		return ""
	}
	var values []string
	if err := json.Unmarshal([]byte(value), &values); err != nil {
		return ""
	}
	return strings.Join(values, " ")
}

func faceTextFromJSON(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "[]" || value == "null" {
		return ""
	}
	var faces []Face
	if err := json.Unmarshal([]byte(value), &faces); err != nil {
		return ""
	}
	return faceText(faces)
}

func matchText(text, value string) bool {
	text = strings.ToLower(text)
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return true
	}
	if strings.Contains(value, "*") {
		return wildcardContains(text, value) || wildcardContains(searchtext.GermanFold(text), searchtext.GermanFold(value))
	}
	return strings.Contains(text, value) || strings.Contains(searchtext.GermanFold(text), searchtext.GermanFold(value))
}

func matchTag(tags []string, value string) bool {
	wanted := cleanPhotoTags([]string{value})
	if len(wanted) == 0 {
		return true
	}
	tags = cleanPhotoTags(tags)
	wantedFolded := searchtext.GermanFold(wanted[0])
	for _, tag := range tags {
		if tag == wanted[0] || searchtext.GermanFold(tag) == wantedFolded {
			return true
		}
	}
	return false
}

func wildcardContains(text, pattern string) bool {
	parts := strings.Split(pattern, "*")
	pos := 0
	for _, part := range parts {
		if part == "" {
			continue
		}
		index := strings.Index(text[pos:], part)
		if index < 0 {
			return false
		}
		pos += index + len(part)
	}
	return true
}

func searchText(media Media) string {
	var b strings.Builder
	b.Grow(len(media.Name) + len(media.Path) + len(media.Directory) + len(media.Camera) + len(media.Lens) + len(media.Orientation) + len(media.Type) + 48)
	appendSearchPart(&b, media.Name)
	appendSearchPart(&b, media.Path)
	appendSearchPart(&b, media.Directory)
	appendSearchPart(&b, media.Camera)
	appendSearchPart(&b, media.Lens)
	appendSearchPart(&b, media.Orientation)
	appendSearchPart(&b, media.Type)
	appendSearchPart(&b, strings.Join(media.Keywords, " "))
	appendSearchPart(&b, strings.Join(media.Tags, " "))
	appendSearchPart(&b, faceText(media.Faces))
	if date := mediaDate(media); !date.IsZero() {
		appendSearchPart(&b, date.Format("2006-01-02"))
		appendSearchPart(&b, strconv.Itoa(date.Year()))
	}
	return b.String()
}

func appendSearchPart(b *strings.Builder, value string) {
	if value == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteByte(' ')
	}
	b.WriteString(value)
}

func faceText(faces []Face) string {
	names := make([]string, 0, len(faces))
	for _, face := range faces {
		names = append(names, face.Name)
	}
	return strings.Join(names, " ")
}
