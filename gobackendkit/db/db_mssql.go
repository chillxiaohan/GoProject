package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"gobackendkit/dialect"

	_ "github.com/microsoft/go-mssqldb"
)

// mssqlDB 封装 SQL Server 的 database/sql.DB，实现 DB 接口
type mssqlDB struct {
	db     *sql.DB
	dbType DBType
}

func newMSSQLDB(config DBConfig) (DB, error) {
	var connString string
	if config.User == "" && config.Password == "" {
		connString = fmt.Sprintf("server=%s;port=%d;database=%s;trusted_connection=yes;encrypt=disable",
			config.Host, config.Port, config.DBName)
	} else {
		connString = fmt.Sprintf("server=%s;port=%d;user id=%s;password=%s;database=%s;encrypt=disable",
			config.Host, config.Port, config.User, config.Password, config.DBName)
	}

	db, err := sql.Open("mssql", connString)
	if err != nil {
		return nil, fmt.Errorf("创建 SQL Server 连接失败: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	err = db.PingContext(context.Background())
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("SQL Server 连接测试失败: %w", err)
	}

	log.Printf("[kit/db] Successfully connected to SQL Server database!")
	return &mssqlDB{db: db, dbType: DBTypeSQLServer}, nil
}

func (db *mssqlDB) Exec(ctx context.Context, query string, args ...interface{}) (Result, error) {
	convertedQuery := dialect.For(dialect.SQLServer).Render(query)
	convertedQuery = convertPlaceholders(convertedQuery)
	result, err := db.db.ExecContext(ctx, convertedQuery, args...)
	if err != nil {
		return nil, err
	}
	return &sqlResult{result: result}, nil
}

func (db *mssqlDB) Query(ctx context.Context, query string, args ...interface{}) (Rows, error) {
	convertedQuery := dialect.For(dialect.SQLServer).Render(query)
	convertedQuery = convertPlaceholders(convertedQuery)
	rows, err := db.db.QueryContext(ctx, convertedQuery, args...)
	if err != nil {
		return nil, err
	}
	return &sqlRows{rows: rows}, nil
}

func (db *mssqlDB) QueryRow(ctx context.Context, query string, args ...interface{}) Row {
	convertedQuery := dialect.For(dialect.SQLServer).Render(query)
	convertedQuery = convertPlaceholders(convertedQuery)
	row := db.db.QueryRowContext(ctx, convertedQuery, args...)
	return &sqlRow{row: row}
}

func (db *mssqlDB) Close() error {
	return db.db.Close()
}

func (db *mssqlDB) Ping(ctx context.Context) error {
	return db.db.PingContext(ctx)
}

func (db *mssqlDB) GetDBType() DBType {
	return db.dbType
}
