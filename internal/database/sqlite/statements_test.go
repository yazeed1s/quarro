package sqlite

import "testing"

func extract(sql string, r StatementRange) string {
	return string([]rune(sql)[r.Start:r.End])
}

func TestSplitStatementsBasic(t *testing.T) {
	sql := "SELECT 1; SELECT 2;"
	ranges := SplitStatements(sql)
	if len(ranges) != 2 {
		t.Fatalf("got %d ranges, want 2: %+v", len(ranges), ranges)
	}
	if extract(sql, ranges[0]) != "SELECT 1" {
		t.Errorf("range 0 = %q", extract(sql, ranges[0]))
	}
	if extract(sql, ranges[1]) != "SELECT 2" {
		t.Errorf("range 1 = %q", extract(sql, ranges[1]))
	}
}

func TestSplitStatementsNoTrailingSemicolon(t *testing.T) {
	ranges := SplitStatements("SELECT 1")
	if len(ranges) != 1 ||
		extract("SELECT 1", ranges[0]) != "SELECT 1" {
		t.Fatalf("got %+v", ranges)
	}
}

func TestSplitStatementsSemicolonInsideString(t *testing.T) {
	sql := "SELECT ';' AS x; SELECT 2;"
	ranges := SplitStatements(sql)
	if len(ranges) != 2 {
		t.Fatalf(
			"got %d ranges, want 2: %+v (%q)",
			len(ranges),
			ranges,
			sqlSlices(sql, ranges),
		)
	}
	if extract(sql, ranges[0]) != "SELECT ';' AS x" {
		t.Errorf("range 0 = %q", extract(sql, ranges[0]))
	}
}

func TestSplitStatementsEscapedQuoteInsideString(t *testing.T) {
	sql := "SELECT 'it''s; still one string' AS x; SELECT 2;"
	ranges := SplitStatements(sql)
	if len(ranges) != 2 {
		t.Fatalf(
			"got %d ranges, want 2: %q",
			len(ranges),
			sqlSlices(sql, ranges),
		)
	}
}

func TestSplitStatementsSemicolonInsideQuotedIdentifier(
	t *testing.T,
) {
	sql := `SELECT 1 AS "weird;name"; SELECT 2;`
	ranges := SplitStatements(sql)
	if len(ranges) != 2 {
		t.Fatalf(
			"got %d ranges, want 2: %q",
			len(ranges),
			sqlSlices(sql, ranges),
		)
	}
}

func TestSplitStatementsSemicolonInsideLineComment(t *testing.T) {
	sql := "SELECT 1; -- a comment with a ; in it\nSELECT 2;"
	ranges := SplitStatements(sql)
	if len(ranges) != 2 {
		t.Fatalf(
			"got %d ranges, want 2: %q",
			len(ranges),
			sqlSlices(sql, ranges),
		)
	}
}

func TestSplitStatementsSemicolonInsideBlockComment(t *testing.T) {
	sql := "SELECT 1; /* a ; comment\nspanning lines */ SELECT 2;"
	ranges := SplitStatements(sql)
	if len(ranges) != 2 {
		t.Fatalf(
			"got %d ranges, want 2: %q",
			len(ranges),
			sqlSlices(sql, ranges),
		)
	}
}

func TestSplitStatementsWhitespaceAndCommentOnlyFragmentsSkipped(
	t *testing.T,
) {
	sql := "SELECT 1;   ;\n-- just a comment\n;  \nSELECT 2;"
	ranges := SplitStatements(sql)
	if len(ranges) != 2 {
		t.Fatalf(
			"got %d ranges, want 2: %q",
			len(ranges),
			sqlSlices(sql, ranges),
		)
	}
}

func TestSplitStatementsEmptyInput(t *testing.T) {
	if ranges := SplitStatements(""); len(ranges) != 0 {
		t.Fatalf("got %+v, want none", ranges)
	}
	if ranges := SplitStatements("   \n\t  "); len(ranges) != 0 {
		t.Fatalf("got %+v, want none", ranges)
	}
}

func TestSplitStatementsBackticksAndBrackets(t *testing.T) {
	sql := "SELECT `a;b` FROM t; SELECT [c;d] FROM t;"
	ranges := SplitStatements(sql)
	if len(ranges) != 2 {
		t.Fatalf(
			"got %d ranges, want 2: %q",
			len(ranges),
			sqlSlices(sql, ranges),
		)
	}
}

func sqlSlices(sql string, ranges []StatementRange) []string {
	out := make([]string, len(ranges))
	for i, r := range ranges {
		out[i] = extract(sql, r)
	}
	return out
}

func TestUTF16Offsets(t *testing.T) {
	ranges := SplitStatements("SELECT '😀'; SELECT 2;")
	if len(ranges) != 2 || ranges[1].Start != 13 {
		t.Fatalf("not UTF-16 offsets: %+v", ranges)
	}
}
