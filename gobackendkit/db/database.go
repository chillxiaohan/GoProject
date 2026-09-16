package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"time"

	"gobackendkit/dialect"
)

// DBType 数据库类型
type DBType string

const (
	DBTypePostgreSQL DBType = "postgresql"
	DBTypeSQLServer  DBType = "sqlserver"
)

// DBConfig 数据库连接参数
type DBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
}

// OnChangeCallback LISTEN/NOTIFY 回调（仅 PostgreSQL）
type OnChangeCallback func(tableName, payload string, timestamp time.Time)

// DB 数据库操作接口，统一封装 PostgreSQL 和 SQL Server 的差异
type DB interface {
	Exec(ctx context.Context, query string, args ...interface{}) (Result, error)
	Query(ctx context.Context, query string, args ...interface{}) (Rows, error)
	QueryRow(ctx context.Context, query string, args ...interface{}) Row
	Close() error
	Ping(ctx context.Context) error
	GetDBType() DBType
}

// Result 执行结果接口
type Result interface {
	RowsAffected() int64
}

// Rows 查询结果行集接口
type Rows interface {
	Scan(dest ...interface{}) error
	Next() bool
	Close() error
	Err() error
}

// Row 单行查询结果接口
type Row interface {
	Scan(dest ...interface{}) error
}

// sqlResult 包装 database/sql.Result 实现 Result 接口
type sqlResult struct {
	result sql.Result
}

func (r *sqlResult) RowsAffected() int64 {
	n, _ := r.result.RowsAffected()
	return n
}

// sqlRows 包装 *sql.Rows 实现 Rows 接口
type sqlRows struct {
	rows *sql.Rows
}

func (r *sqlRows) Scan(dest ...interface{}) error {
	return r.rows.Scan(dest...)
}

func (r *sqlRows) Next() bool {
	return r.rows.Next()
}

func (r *sqlRows) Close() error {
	return r.rows.Close()
}

func (r *sqlRows) Err() error {
	return r.rows.Err()
}

// sqlRow 包装 *sql.Row 实现 Row 接口
type sqlRow struct {
	row *sql.Row
}

func (r *sqlRow) Scan(dest ...interface{}) error {
	return r.row.Scan(dest...)
}

// placeholderRegex 匹配 PostgreSQL 的 $N 占位符
var placeholderRegex = regexp.MustCompile(`\$(\d+)`)

// convertPlaceholders 将 PostgreSQL 的 $1, $2 占位符转换为 SQL Server 的 @p1, @p2 格式
func convertPlaceholders(query string) string {
	return placeholderRegex.ReplaceAllStringFunc(query, func(match string) string {
		numStr := match[1:]
		if _, err := strconv.Atoi(numStr); err == nil {
			return "@p" + numStr
		}
		return match
	})
}

func convertPlaceholdersForPostgres(query string) string {
	return query
}

// NewDB 根据配置创建数据库连接
func NewDB(dbType DBType, config DBConfig) (DB, error) {
	switch dbType {
	case DBTypePostgreSQL:
		return newPostgresDB(config)
	case DBTypeSQLServer:
		return newMSSQLDB(config)
	default:
		return nil, fmt.Errorf("不支持的数据库类型: %s", dbType)
	}
}

// InitDB 初始化数据库连接（兼容旧接口，默认使用 PostgreSQL）
func InitDB(dbConfig DBConfig) DB {
	dbType := DBTypePostgreSQL
	if definesDBType != "" {
		dbType = DBType(definesDBType)
	}
	db, err := NewDB(dbType, dbConfig)
	if err != nil {
		log.Printf("[kit/db] 数据库连接失败: %v", err)
		return nil
	}
	return db
}

var definesDBType string

// SetDBType 设置全局数据库类型（同步 dialect 方言枚举）。
func SetDBType(dbType string) {
	definesDBType = dbType
	dialect.SetKind(dialect.Kind(dbType))
}

// GetDBType 获取全局数据库类型
func GetDBType() string {
	if definesDBType == "" {
		return string(DBTypePostgreSQL)
	}
	return definesDBType
}

// IsPostgreSQL 判断当前是否为 PostgreSQL
func IsPostgreSQL() bool {
	return GetDBType() == string(DBTypePostgreSQL)
}

// IsSQLServer 判断当前是否为 SQL Server
func IsSQLServer() bool {
	return GetDBType() == string(DBTypeSQLServer)
}

// ConvertQuery 根据数据库类型转换 SQL 查询中的占位符
func ConvertQuery(query string) string {
	if IsSQLServer() {
		return convertPlaceholders(query)
	}
	return convertPlaceholdersForPostgres(query)
}

// ConvertQueryWithDBType 根据指定数据库类型转换 SQL 查询中的占位符
func ConvertQueryWithDBType(query string, dbType DBType) string {
	if dbType == DBTypeSQLServer {
		return convertPlaceholders(query)
	}
	return convertPlaceholdersForPostgres(query)
}

// GetTableNames 查询当前数据库中所有的普通表
func GetTableNames(db DB) ([]string, error) {
	var query string
	if db.GetDBType() == DBTypeSQLServer {
		query = `
			SELECT TABLE_NAME
			FROM INFORMATION_SCHEMA.TABLES
			WHERE TABLE_TYPE = 'BASE TABLE'
			ORDER BY TABLE_NAME
		`
	} else {
		query = `
			SELECT table_name
			FROM information_schema.tables
			WHERE table_schema = 'public'
			  AND table_type = 'BASE TABLE'
			ORDER BY table_name
		`
	}

	rows, err := db.Query(context.Background(), query)
	if err != nil {
		return nil, fmt.Errorf("执行查询失败: %w", err)
	}
	defer rows.Close()

	var tableNames []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, fmt.Errorf("读取表名失败: %w", err)
		}
		tableNames = append(tableNames, tableName)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历结果出错: %w", err)
	}

	return tableNames, nil
}
