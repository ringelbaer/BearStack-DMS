package photos

import "testing"

func TestQueryExpressionFeedsMatcherAndIndexPlan(t *testing.T) {
	query := `tag:urlaub and camera:sony 2-of:(lens:prime,person:ada)`
	expression := parseQueryExpression(query)
	if expression.HasOR || len(expression.Groups) != 1 || len(expression.Groups[0]) != 4 {
		t.Fatalf("expression = %#v", expression)
	}
	if !expression.Groups[0][1].Skip {
		t.Fatalf("and connector was not marked as skip: %#v", expression.Groups[0][1])
	}
	if expression.Groups[0][3].NOf != 2 || len(expression.Groups[0][3].NOfTerms) != 2 {
		t.Fatalf("n-of node = %#v", expression.Groups[0][3])
	}

	compiled := compileMediaQuery(query)
	if len(compiled.groups) != 1 || len(compiled.groups[0]) != 4 {
		t.Fatalf("compiled query = %#v", compiled)
	}
	plan := indexQueryPlanFor(query)
	if !plan.PostFilter {
		t.Fatal("n-of term should force post-filtering")
	}
	if len(plan.SQLTerms) != 1 || plan.SQLTerms[0].Field != "tag" {
		t.Fatalf("sql terms = %#v", plan.SQLTerms)
	}
	if plan.FTSQuery != `"sony"` {
		t.Fatalf("fts query = %q", plan.FTSQuery)
	}
}

func TestDisjunctiveQueryPlanUsesExpressionGroups(t *testing.T) {
	plan := indexQueryPlanFor(`summer or tag:winter`)
	if !plan.Disjunctive || !plan.PostFilter {
		t.Fatalf("plan = %#v", plan)
	}
	if plan.FTSQuery != `"summer" OR "winter"` {
		t.Fatalf("fts query = %q", plan.FTSQuery)
	}
}

func TestIndexQueryPlanKeepsSimpleNegationsInSQL(t *testing.T) {
	plan := indexQueryPlanFor(`gps:false resolution:>=12 -type:video`)
	if plan.PostFilter {
		t.Fatalf("plan should not need post-filtering: %#v", plan)
	}
	if len(plan.SQLTerms) != 3 {
		t.Fatalf("sql terms = %#v", plan.SQLTerms)
	}
	if plan.SQLTerms[2].Field != "type" || !plan.SQLTerms[2].Negated {
		t.Fatalf("negated type term = %#v", plan.SQLTerms[2])
	}
}
