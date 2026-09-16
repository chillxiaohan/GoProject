package dialect

// Kind 数据库方言枚举，与 kit/db.DBType / defines.DBType 配置值一致。
//
// 用法：
//
//	d := dialect.Current()                 // 当前配置的库
//	d := dialect.For(dialect.SQLServer)    // 指定枚举切换
//
// 业务 SQL 推荐写法：
//  1. 整句 T-SQL 风格 + Db 执行（PG 驱动自动 Render）
//  2. 片段用 d.Now() / d.NotDeleted("fc") / d.BoolBitEq("is_active", true)
//  3. DDL 用 d.ConvertDDL(mssqlDDL)
//  4. 元数据用 d.Meta().TableExists
type Kind string

const (
	PostgreSQL Kind = "postgresql"
	SQLServer  Kind = "sqlserver"
)

var currentKind Kind = PostgreSQL

// SetKind 设置当前进程方言（main 启动时与 DBType 同步调用）。
func SetKind(kind Kind) {
	currentKind = kind
}

// GetKind 返回当前方言枚举。
func GetKind() Kind {
	return currentKind
}

// For 按枚举获取方言实例。
func For(kind Kind) Dialect {
	switch kind {
	case SQLServer:
		return sqlServerDialect{}
	default:
		return postgresDialect{}
	}
}

// Current 返回当前进程配置的数据库方言。
func Current() Dialect {
	return For(currentKind)
}

// IsPostgreSQL 当前是否为 PostgreSQL。
func IsPostgreSQL() bool {
	return Current().Kind() == PostgreSQL
}

// SQL 将 T-SQL 风格查询渲染为当前方言（Handler 一般经 Db 自动转换，无需手动调用）。
func SQL(query string) string {
	return Current().Render(query)
}

// IsSQLServer 当前是否为 SQL Server。
func IsSQLServer() bool {
	return Current().Kind() == SQLServer
}
