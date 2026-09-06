// Datei plant Fotoindex-Suchabfragen und kapselt Suchterm-Hilfsfunktionen.
package photos

import (
	"fmt"
	"strconv"
	"strings"

	"bearstack/internal/boolutil"
	"bearstack/internal/searchtext"
)

type indexQueryPlan struct {
	ExpressionSQL  string
	ExpressionArgs []any
	SQLTerms       []queryTerm
	FTSQuery       string
	PostFilter     bool
	Disjunctive    bool
}

func indexQueryPlanFor(query string) indexQueryPlan {
	expression := parseQueryExpression(query)
	plan := indexQueryPlan{}
	if queryHasPerson(query) {
		if sql, args, ok := personExpressionSQL(expression); ok {
			plan.ExpressionSQL = sql
			plan.ExpressionArgs = args
			return plan
		}
	}
	if expression.HasOR {
		plan.PostFilter = true
		plan.Disjunctive = true
		plan.FTSQuery = disjunctiveMediaFTSQueryForExpression(expression)
		return plan
	}
	ftsTerms := make([]string, 0, len(expression.Groups))
	for _, group := range expression.Groups {
		for _, node := range group {
			if node.Skip {
				continue
			}
			if node.NOf > 0 {
				plan.PostFilter = true
				continue
			}
			applyIndexQueryNode(&plan, &ftsTerms, node.Term)
		}
	}
	if len(ftsTerms) > 0 {
		plan.FTSQuery = strings.Join(ftsTerms, " AND ")
	}
	return plan
}

func applyIndexQueryNode(plan *indexQueryPlan, ftsTerms *[]string, term queryTerm) {
	if term.Negated {
		if indexTermCanUseSQL(term) {
			plan.SQLTerms = append(plan.SQLTerms, term)
		} else {
			plan.PostFilter = true
		}
		return
	}
	switch term.Field {
	case "type", "gps", "directory", "file_name", "date", "orientation", "resolution", "tag":
		plan.SQLTerms = append(plan.SQLTerms, term)
	case "person":
		plan.SQLTerms = append(plan.SQLTerms, term)
	case "":
		if fts, safe := searchtext.FTSQueryTerm(term.Value); safe && fts != "" {
			*ftsTerms = append(*ftsTerms, fts)
		} else if !safe {
			plan.PostFilter = true
		}
	default:
		if fts, safe := searchtext.FTSQueryTerm(term.Value); safe && fts != "" {
			*ftsTerms = append(*ftsTerms, fts)
		} else if !safe {
			plan.PostFilter = true
		}
	}
}

func indexTermCanUseSQL(term queryTerm) bool {
	switch term.Field {
	case "type", "gps", "directory", "file_name", "date", "orientation", "resolution", "tag", "person":
		return true
	default:
		return false
	}
}

func disjunctiveMediaFTSQueryForExpression(expression queryExpression) string {
	if len(expression.Groups) == 0 {
		return ""
	}
	ftsGroups := make([]string, 0, len(expression.Groups))
	for _, group := range expression.Groups {
		terms := make([]string, 0, len(group))
		for _, node := range group {
			if node.Skip || node.NOf > 0 {
				continue
			}
			term := node.Term
			if term.Field == "person" {
				return ""
			}
			if term.Negated || !mediaSearchIncludesField(term.Field) {
				continue
			}
			if fts, safe := searchtext.FTSQueryTerm(term.Value); safe && fts != "" {
				terms = append(terms, fts)
			}
		}
		if len(terms) == 0 {
			return ""
		}
		if len(terms) == 1 {
			ftsGroups = append(ftsGroups, terms[0])
		} else {
			ftsGroups = append(ftsGroups, "("+strings.Join(terms, " AND ")+")")
		}
	}
	if len(ftsGroups) == 1 {
		return ftsGroups[0]
	}
	return strings.Join(ftsGroups, " OR ")
}

func mediaSearchIncludesField(field string) bool {
	switch field {
	case "gps", "resolution", "person":
		return false
	default:
		return true
	}
}

func truthyPhotoValue(value string) bool {
	return boolutil.Truthy(value)
}

func falseyPhotoValue(value string) bool {
	return boolutil.Falsey(value)
}

func yearPrefix(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) != 4 {
		return "", false
	}
	if _, err := strconv.Atoi(value); err != nil {
		return "", false
	}
	return value, true
}

func nextYear(value string) string {
	year, err := strconv.Atoi(value)
	if err != nil {
		return value
	}
	return fmt.Sprintf("%04d", year+1)
}

func folderSearchNeedsFTS(plan indexQueryPlan) bool {
	if plan.FTSQuery != "" {
		return true
	}
	for _, term := range plan.SQLTerms {
		switch term.Field {
		case "tag":
			continue
		default:
			return true
		}
	}
	return false
}

func folderSearchFTSQuery(plan indexQueryPlan, fallback string) string {
	if plan.FTSQuery != "" {
		return plan.FTSQuery
	}
	terms := make([]string, 0, len(plan.SQLTerms))
	for _, term := range plan.SQLTerms {
		if term.Field == "tag" || term.Value == "" {
			continue
		}
		if fts, safe := searchtext.FTSQueryTerm(term.Value); safe && fts != "" {
			terms = append(terms, fts)
		}
	}
	if len(terms) > 0 {
		return strings.Join(terms, " AND ")
	}
	if fts, safe := searchtext.FTSQueryTerm(fallback); safe {
		return fts
	}
	return ""
}

func blogSearchNeedsFTS(plan indexQueryPlan) bool {
	if plan.FTSQuery != "" {
		return true
	}
	for _, term := range plan.SQLTerms {
		switch term.Field {
		case "tag", "directory", "file_name", "type":
			continue
		default:
			return true
		}
	}
	return false
}

func blogSearchFTSQuery(plan indexQueryPlan, fallback string) string {
	if plan.FTSQuery != "" {
		return plan.FTSQuery
	}
	terms := make([]string, 0, len(plan.SQLTerms))
	for _, term := range plan.SQLTerms {
		switch term.Field {
		case "tag", "directory", "file_name", "type":
			continue
		default:
			if term.Value != "" {
				if fts, safe := searchtext.FTSQueryTerm(term.Value); safe && fts != "" {
					terms = append(terms, fts)
				}
			}
		}
	}
	if len(terms) > 0 {
		return strings.Join(terms, " AND ")
	}
	if fts, safe := searchtext.FTSQueryTerm(fallback); safe {
		return fts
	}
	return ""
}

func prefixRange(prefix string) (string, string) {
	if prefix == "" {
		return "", string(rune(0x10ffff))
	}
	runes := []rune(prefix)
	runes[len(runes)-1]++
	return prefix, string(runes)
}

// Compound person filters stay in SQL so OR/N-of searches do not materialize
// the photo collection or hit the legacy post-filter candidate limit.
func personExpressionSQL(expression queryExpression) (string, []any, bool) {
	groups := []string{}
	args := []any{}
	termSQL := func(t queryTerm) (string, []any, bool) {
		if !indexTermCanUseSQL(t) {
			return "", nil, false
		}
		where := []string{}
		values := []any{}
		appendIndexTermWhere(&where, &values, t)
		if len(where) == 0 {
			return "", nil, false
		}
		return "(" + strings.Join(where, " AND ") + ")", values, true
	}
	for _, group := range expression.Groups {
		conditions := []string{}
		for _, node := range group {
			if node.Skip {
				continue
			}
			if node.NOf > 0 {
				counts := []string{}
				for _, t := range node.NOfTerms {
					sql, a, ok := termSQL(t)
					if !ok {
						return "", nil, false
					}
					counts = append(counts, "CASE WHEN "+sql+" THEN 1 ELSE 0 END")
					args = append(args, a...)
				}
				if len(counts) == 0 {
					return "", nil, false
				}
				conditions = append(conditions, "("+strings.Join(counts, " + ")+" >= "+strconv.Itoa(node.NOf)+")")
			} else {
				sql, a, ok := termSQL(node.Term)
				if !ok {
					return "", nil, false
				}
				conditions = append(conditions, sql)
				args = append(args, a...)
			}
		}
		if len(conditions) == 0 {
			return "", nil, false
		}
		groups = append(groups, "("+strings.Join(conditions, " AND ")+")")
	}
	if len(groups) == 0 {
		return "", nil, false
	}
	return "(" + strings.Join(groups, " OR ") + ")", args, true
}
