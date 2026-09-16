package dialect

import (
	"strings"
	"testing"
)

func TestPostgresRenderBoolAndPage(t *testing.T) {
	SetKind(PostgreSQL)
	d := For(PostgreSQL)

	q := d.Render(`SELECT id FROM t WHERE is_delete = 0 AND is_active = 1 ORDER BY id OFFSET ?1 ROWS FETCH NEXT ?2 ROWS ONLY`)
	if !strings.Contains(q, "is_delete = false") || !strings.Contains(q, "is_active = true") {
		t.Fatalf("bool conversion failed: %s", q)
	}
	if !strings.Contains(q, "LIMIT") || !strings.Contains(q, "OFFSET") {
		t.Fatalf("pagination conversion failed: %s", q)
	}
}

func TestSQLServerRenderPassthrough(t *testing.T) {
	d := For(SQLServer)
	in := `SELECT id FROM t WHERE is_delete = 0`
	if out := d.Render(in); out != in {
		t.Fatalf("expected passthrough, got %s", out)
	}
}

func TestDialectFragments(t *testing.T) {
	pg := For(PostgreSQL)
	mssql := For(SQLServer)

	if pg.Now() != "CURRENT_TIMESTAMP" || mssql.Now() != "GETDATE()" {
		t.Fatal("Now() mismatch")
	}
	if pg.NotDeleted("fc") != "fc.is_delete = false" || mssql.NotDeleted("fc") != "fc.is_delete = 0" {
		t.Fatal("NotDeleted() mismatch")
	}
	if pg.BoolBitLiteral(true) != "true" || mssql.BoolBitLiteral(true) != "1" {
		t.Fatal("BoolBitLiteral() mismatch")
	}
	if pg.IntFlagEq("is_completed", 0) != "is_completed = 0" {
		t.Fatal("IntFlagEq should stay integer on PG")
	}
}

func TestCurrentUsesGlobalDBType(t *testing.T) {
	SetKind(SQLServer)
	if !IsSQLServer() || IsPostgreSQL() {
		t.Fatal("Current() should follow SetKind")
	}
	SetKind(PostgreSQL)
	if !IsPostgreSQL() {
		t.Fatal("expected PostgreSQL after reset")
	}
}
