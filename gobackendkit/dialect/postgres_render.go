package dialect

import (
	"regexp"
	"strconv"
	"strings"
)

// pgBoolColumn 匹配 T-SQL BIT 风格列名（is_*），排除整型/字符串伪布尔列。
const pgBoolColumn = `(?:\w+\.)?is_\w+`

var (
	rePGPlaceholder       = regexp.MustCompile(`\?(\d+)`)
	rePGBarePlaceholder   = regexp.MustCompile(`\?`)
	rePGISNULL            = regexp.MustCompile(`(?i)\bISNULL\s*\(`)
	rePGGetDate           = regexp.MustCompile(`(?i)\bGETDATE\s*\(\s*\)`)
	rePGSysUTC            = regexp.MustCompile(`(?i)\bSYSUTCDATETIME\s*\(\s*\)`)
	rePGUnicodeStr        = regexp.MustCompile(`\bN'`)
	rePGSelectTop         = regexp.MustCompile(`(?is)^(\s*SELECT)\s+TOP\s+(\d+)\s+`)
	rePGIdentityPK        = regexp.MustCompile(`(?i)INT\s+IDENTITY\s*\(\s*1\s*,\s*1\s*\)\s+PRIMARY\s+KEY`)
	rePGNVarcharMax       = regexp.MustCompile(`(?i)NVARCHAR\s*\(\s*MAX\s*\)`)
	rePGNVarchar          = regexp.MustCompile(`(?i)NVARCHAR\s*\(\s*(\d+)\s*\)`)
	rePGDateTime2         = regexp.MustCompile(`(?i)DATETIME2`)
	rePGDateTime          = regexp.MustCompile(`(?i)\bDATETIME\b`)
	rePGBitDefault0       = regexp.MustCompile(`(?i)BIT\s+NOT\s+NULL\s+DEFAULT\s+0`)
	rePGBitNotNull        = regexp.MustCompile(`(?i)BIT\s+NOT\s+NULL`)
	rePGAddBit            = regexp.MustCompile(`(?i)ADD\s+(\w+)\s+BIT\s+NOT\s+NULL\s+CONSTRAINT\s+\w+\s+DEFAULT\s+0`)
	rePGAddNVarchar       = regexp.MustCompile(`(?i)ADD\s+(\w+)\s+NVARCHAR\s*\(\s*(\d+)\s*\)`)
	rePGAddNVarcharMax    = regexp.MustCompile(`(?i)ADD\s+(\w+)\s+NVARCHAR\s*\(\s*MAX\s*\)`)
	rePGConstraintDefault = regexp.MustCompile(`(?i)\s+CONSTRAINT\s+\w+\s+DEFAULT\s+[^,\)]+`)
	rePGBoolColEq0        = regexp.MustCompile(`(?i)\b(` + pgBoolColumn + `)\s*=\s*0\b`)
	rePGBoolColEq1        = regexp.MustCompile(`(?i)\b(` + pgBoolColumn + `)\s*=\s*1\b`)
	rePGCoalesceBool0     = regexp.MustCompile(`(?i)COALESCE\s*\(\s*(` + pgBoolColumn + `)\s*,\s*0\s*\)`)
	rePGCoalesceEq0       = regexp.MustCompile(`(?i)(COALESCE\s*\(\s*(?:\w+\.)?is_\w+[^)]*\))\s*=\s*0\b`)
	rePGInsertTplTrue     = regexp.MustCompile(`(?i)(VALUES\s*\(\s*\$\d+\s*,\s*\$\d+\s*,\s*\$\d+\s*,)\s*1\s*,`)
	rePGInsertTailZero    = regexp.MustCompile(`,\s*0\s*\)\s*$`)
	rePGOffsetFetch       = regexp.MustCompile(`(?i)\s+OFFSET\s+(\?\d+|\$\d+|\d+)\s+ROWS\s+FETCH\s+NEXT\s+(\?\d+|\$\d+|\d+)\s+ROWS\s+ONLY`)
	rePGCastNVarchar      = regexp.MustCompile(`(?i)CAST\s*\(\s*([^)]+?)\s+AS\s+NVARCHAR\s*\(\s*(\d+)\s*\)\)`)
	rePGCoalesceContent   = regexp.MustCompile(`(?i)COALESCE\s*\(\s*content\s*,\s*''\s*\)`)
	rePGCoalesceInnerCol  = regexp.MustCompile(`(?i)(?:\w+\.)?(is_\w+)`)
)

func normalizePostgresQuery(query string) string {
	q := strings.TrimSpace(query)
	if q == "" {
		return q
	}

	if m := rePGSelectTop.FindStringSubmatch(q); len(m) >= 3 {
		q = rePGSelectTop.ReplaceAllString(q, m[1]+" ")
		upper := strings.ToUpper(q)
		if !strings.Contains(upper, " LIMIT ") {
			q = strings.TrimRight(q, "; \t\r\n") + " LIMIT " + m[2]
		}
	}

	q = rePGPlaceholder.ReplaceAllString(q, "$$$1")
	q = rePGISNULL.ReplaceAllString(q, "COALESCE(")
	q = rePGCoalesceContent.ReplaceAllString(q, "COALESCE(content::text, '')")
	q = rePGGetDate.ReplaceAllString(q, "CURRENT_TIMESTAMP")
	q = rePGSysUTC.ReplaceAllString(q, "CURRENT_TIMESTAMP")
	q = rePGUnicodeStr.ReplaceAllString(q, "'")
	q = replacePGBoolComparisons(q)
	q = rePGInsertTplTrue.ReplaceAllString(q, "${1} true,")
	if strings.Contains(strings.ToUpper(q), "IS_DELETE") || strings.Contains(strings.ToUpper(q), "IS_ACTIVE") {
		q = rePGInsertTailZero.ReplaceAllString(q, ", false)")
	}

	if strings.Contains(q, "?") && !strings.Contains(q, "$") {
		i := 1
		q = rePGBarePlaceholder.ReplaceAllStringFunc(q, func(string) string {
			s := "$" + strconv.Itoa(i)
			i++
			return s
		})
	}

	q = rePGOffsetFetch.ReplaceAllStringFunc(q, func(match string) string {
		sub := rePGOffsetFetch.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		return " LIMIT " + sub[2] + " OFFSET " + sub[1]
	})
	q = rePGCastNVarchar.ReplaceAllString(q, "CAST($1 AS VARCHAR($2))")

	return q
}

func convertTSQLDDLToPostgres(ddl string) string {
	q := ddl
	q = rePGIdentityPK.ReplaceAllString(q, "SERIAL PRIMARY KEY")
	q = rePGNVarcharMax.ReplaceAllString(q, "TEXT")
	q = rePGNVarchar.ReplaceAllString(q, "VARCHAR($1)")
	q = rePGDateTime2.ReplaceAllString(q, "TIMESTAMP")
	q = rePGDateTime.ReplaceAllString(q, "TIMESTAMP")
	q = rePGBitDefault0.ReplaceAllString(q, "BOOLEAN NOT NULL DEFAULT FALSE")
	q = rePGBitNotNull.ReplaceAllString(q, "BOOLEAN NOT NULL DEFAULT FALSE")
	q = rePGGetDate.ReplaceAllString(q, "CURRENT_TIMESTAMP")
	q = rePGSysUTC.ReplaceAllString(q, "CURRENT_TIMESTAMP")
	q = rePGUnicodeStr.ReplaceAllString(q, "'")
	q = rePGAddBit.ReplaceAllString(q, "ADD COLUMN $1 BOOLEAN NOT NULL DEFAULT FALSE")
	q = rePGAddNVarcharMax.ReplaceAllString(q, "ADD COLUMN $1 TEXT")
	q = rePGAddNVarchar.ReplaceAllString(q, "ADD COLUMN $1 VARCHAR($2)")
	q = rePGConstraintDefault.ReplaceAllString(q, "")
	q = strings.ReplaceAll(q, "ADD COLUMN COLUMN", "ADD COLUMN")
	q = replacePGBoolComparisons(q)
	return q
}

func isPGSQLBoolColumn(col string) bool {
	col = strings.ToLower(strings.TrimSpace(col))
	if i := strings.LastIndex(col, "."); i >= 0 {
		col = col[i+1:]
	}
	if !strings.HasPrefix(col, "is_") {
		return false
	}
	switch col {
	case "is_completed", "is_selected":
		return false
	default:
		return true
	}
}

func replacePGBoolComparisons(q string) string {
	q = rePGBoolColEq0.ReplaceAllStringFunc(q, func(m string) string {
		sub := rePGBoolColEq0.FindStringSubmatch(m)
		if len(sub) < 2 || !isPGSQLBoolColumn(sub[1]) {
			return m
		}
		return sub[1] + " = false"
	})
	q = rePGBoolColEq1.ReplaceAllStringFunc(q, func(m string) string {
		sub := rePGBoolColEq1.FindStringSubmatch(m)
		if len(sub) < 2 || !isPGSQLBoolColumn(sub[1]) {
			return m
		}
		return sub[1] + " = true"
	})
	q = rePGCoalesceBool0.ReplaceAllStringFunc(q, func(m string) string {
		sub := rePGCoalesceBool0.FindStringSubmatch(m)
		if len(sub) < 2 || !isPGSQLBoolColumn(sub[1]) {
			return m
		}
		return "COALESCE(" + sub[1] + ", false)"
	})
	q = rePGCoalesceEq0.ReplaceAllStringFunc(q, func(m string) string {
		sub := rePGCoalesceEq0.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		inner := sub[1]
		colSub := rePGCoalesceInnerCol.FindStringSubmatch(inner)
		if len(colSub) < 2 || !isPGSQLBoolColumn(colSub[1]) {
			return m
		}
		return inner + " = false"
	})
	return q
}
