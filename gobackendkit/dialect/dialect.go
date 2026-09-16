package dialect

// Dialect 跨库 SQL 方言：业务层统一写 T-SQL 风格片段，由 Render 在 PG 上自动转换。
type Dialect interface {
	Kind() Kind
	// Render 将 T-SQL 风格整句 SQL 渲染为当前方言可执行语句（占位符保持 ?1 ?2…）。
	Render(query string) string
	// ConvertDDL 将 CREATE/ALTER 从 T-SQL 转为当前方言。
	ConvertDDL(ddl string) string

	Now() string
	IsNull(expr, fallback string) string
	// BoolBitEq 用于 BIT/BOOLEAN 列（is_delete、is_active、is_dept_leader 等）。
	BoolBitEq(column string, value bool) string
	// BoolBitLiteral INSERT/VALUES 中的布尔字面量。
	BoolBitLiteral(value bool) string
	NotDeleted(alias string) string
	// IntFlagEq 用于整型 0/1 标志列（如 user_todos.is_completed）。
	IntFlagEq(column string, value int) string
	// PageClause T-SQL 分页片段；PG 由 Render 转为 LIMIT/OFFSET。
	PageClause(offsetArg, limitArg int) string
	Placeholder(n int) string
	// Meta information_schema 内省语句。
	Meta() Schema
}

type postgresDialect struct{}

func (postgresDialect) Kind() Kind { return PostgreSQL }

func (postgresDialect) Render(query string) string {
	return normalizePostgresQuery(query)
}

func (postgresDialect) ConvertDDL(ddl string) string {
	return convertTSQLDDLToPostgres(ddl)
}

func (postgresDialect) Now() string { return "CURRENT_TIMESTAMP" }

func (postgresDialect) IsNull(expr, fallback string) string {
	return "COALESCE(" + expr + ", " + fallback + ")"
}

func (postgresDialect) BoolBitEq(column string, value bool) string {
	if value {
		return column + " = true"
	}
	return column + " = false"
}

func (postgresDialect) BoolBitLiteral(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func (postgresDialect) NotDeleted(alias string) string {
	col := "is_delete"
	if alias != "" {
		col = alias + ".is_delete"
	}
	return col + " = false"
}

func (postgresDialect) IntFlagEq(column string, value int) string {
	return column + " = " + itoa(value)
}

func (postgresDialect) PageClause(offsetArg, limitArg int) string {
	return "OFFSET ?" + itoa(offsetArg) + " ROWS FETCH NEXT ?" + itoa(limitArg) + " ROWS ONLY"
}

func (postgresDialect) Placeholder(n int) string {
	return "?" + itoa(n)
}

func (postgresDialect) Meta() Schema {
	return Schema{
		TableExists: `
SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = 'public' AND table_name = $1`,
		ColumnExists: `
SELECT COUNT(*) FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2`,
	}
}

type sqlServerDialect struct{}

func (sqlServerDialect) Kind() Kind { return SQLServer }

func (sqlServerDialect) Render(query string) string { return query }

func (sqlServerDialect) ConvertDDL(ddl string) string { return ddl }

func (sqlServerDialect) Now() string { return "GETDATE()" }

func (sqlServerDialect) IsNull(expr, fallback string) string {
	return "ISNULL(" + expr + ", " + fallback + ")"
}

func (sqlServerDialect) BoolBitEq(column string, value bool) string {
	if value {
		return column + " = 1"
	}
	return column + " = 0"
}

func (sqlServerDialect) BoolBitLiteral(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func (sqlServerDialect) NotDeleted(alias string) string {
	col := "is_delete"
	if alias != "" {
		col = alias + ".is_delete"
	}
	return col + " = 0"
}

func (sqlServerDialect) IntFlagEq(column string, value int) string {
	return column + " = " + itoa(value)
}

func (sqlServerDialect) PageClause(offsetArg, limitArg int) string {
	return "OFFSET ?" + itoa(offsetArg) + " ROWS FETCH NEXT ?" + itoa(limitArg) + " ROWS ONLY"
}

func (sqlServerDialect) Placeholder(n int) string {
	return "?" + itoa(n)
}

func (sqlServerDialect) Meta() Schema {
	return Schema{
		TableExists:  `SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_NAME = ?1`,
		ColumnExists: `SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_NAME = ?1 AND COLUMN_NAME = ?2`,
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
