package dialect

import (
	"strings"
	"testing"
)

func TestNormalizePostgresQueryOffsetFetch(t *testing.T) {
	in := `SELECT id FROM field_content WHERE is_delete = 0 AND id > ?1 ORDER BY id ASC OFFSET 0 ROWS FETCH NEXT 1000 ROWS ONLY`
	out := normalizePostgresQuery(in)
	if !strings.Contains(out, "LIMIT 1000") || !strings.Contains(out, "OFFSET 0") || !strings.Contains(out, "is_delete = false") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestNormalizePostgresQueryBoolColumns(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
		not  []string
	}{
		{
			name: "is_active select",
			in:   `SELECT id FROM delivery_feature_templates WHERE is_delete = 0 AND is_active = 1`,
			want: []string{"is_delete = false", "is_active = true"},
		},
		{
			name: "is_completed stays integer",
			in:   `SELECT id FROM user_todos WHERE is_delete = 0 AND ISNULL(is_completed,0) = 0`,
			want: []string{"is_delete = false", "COALESCE(is_completed,0) = 0"},
			not:  []string{"is_completed = false"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := normalizePostgresQuery(tc.in)
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Fatalf("expected %q in:\n%s", w, out)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(out, n) {
					t.Fatalf("did not expect %q in:\n%s", n, out)
				}
			}
		})
	}
}

func TestConvertTSQLDDLToPostgresBoolIndex(t *testing.T) {
	in := `CREATE UNIQUE INDEX uq ON project_deliveries(project_id) WHERE is_delete = 0`
	out := convertTSQLDDLToPostgres(in)
	if !strings.Contains(out, "is_delete = false") {
		t.Fatalf("unexpected output: %s", out)
	}
}
