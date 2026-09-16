package db

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"gobackendkit/dialect"
)

// postgresDB 封装 PostgreSQL 的 pgxpool.Pool，实现 DB 接口
type postgresDB struct {
	pool   *pgxpool.Pool
	dbType DBType
}

func newPostgresDB(config DBConfig) (DB, error) {
	// 勿在 options 里设 lc_messages：PG15+ 非超级用户无权 SET，部署账号 erp_app 会直接连不上
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		config.Host, config.Port, config.User, config.Password, config.DBName)

	pool, err := pgxpool.New(context.Background(), psqlInfo)
	if err != nil {
		return nil, fmt.Errorf("创建 PostgreSQL 连接池失败: %w", err)
	}

	err = pool.Ping(context.Background())
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("PostgreSQL 连接测试失败: %w", err)
	}

	log.Printf("[kit/db] Successfully connected to PostgreSQL database!")
	return &postgresDB{pool: pool, dbType: DBTypePostgreSQL}, nil
}

func (db *postgresDB) Exec(ctx context.Context, query string, args ...interface{}) (Result, error) {
	query = dialect.For(dialect.PostgreSQL).Render(query)
	commandTag, err := db.pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &pgxResult{commandTag: commandTag}, nil
}

func (db *postgresDB) Query(ctx context.Context, query string, args ...interface{}) (Rows, error) {
	query = dialect.For(dialect.PostgreSQL).Render(query)
	rows, err := db.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &pgxRows{rows: rows}, nil
}

func (db *postgresDB) QueryRow(ctx context.Context, query string, args ...interface{}) Row {
	query = dialect.For(dialect.PostgreSQL).Render(query)
	row := db.pool.QueryRow(ctx, query, args...)
	return &pgxRow{row: row}
}

func (db *postgresDB) Close() error {
	db.pool.Close()
	return nil
}

func (db *postgresDB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

func (db *postgresDB) GetDBType() DBType {
	return db.dbType
}

// GetPool 获取原始 pgxpool.Pool（用于 PostgreSQL 特有功能如 LISTEN/NOTIFY）
func (db *postgresDB) GetPool() *pgxpool.Pool {
	return db.pool
}

type pgxResult struct {
	commandTag pgconn.CommandTag
}

func (r *pgxResult) RowsAffected() int64 {
	return r.commandTag.RowsAffected()
}

type pgxRows struct {
	rows pgx.Rows
}

func (r *pgxRows) Scan(dest ...interface{}) error {
	return r.rows.Scan(dest...)
}

func (r *pgxRows) Next() bool {
	return r.rows.Next()
}

func (r *pgxRows) Close() error {
	r.rows.Close()
	return nil
}

func (r *pgxRows) Err() error {
	return r.rows.Err()
}

type pgxRow struct {
	row pgx.Row
}

func (r *pgxRow) Scan(dest ...interface{}) error {
	return r.row.Scan(dest...)
}

// ListenForNotifications 使用连接池监听通知（仅 PostgreSQL 支持）
func ListenForNotifications(ctx context.Context, db DB, wg *sync.WaitGroup, channel string, onChange OnChangeCallback) {
	defer wg.Done()

	if db.GetDBType() != DBTypePostgreSQL {
		log.Printf("[kit/db] 数据库类型 %s 不支持 LISTEN/NOTIFY 功能", db.GetDBType())
		return
	}

	pgDB, ok := db.(*postgresDB)
	if !ok {
		log.Printf("[kit/db] 数据库实例不是 PostgreSQL 类型")
		return
	}

	pool := pgDB.GetPool()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		log.Printf("[kit/db] 获取监听连接失败: %v", err)
		return
	}
	defer conn.Release()

	_, err = conn.Exec(ctx, "LISTEN "+channel)
	if err != nil {
		log.Printf("[kit/db] 监听频道失败: %v,%v", channel, err)
		return
	}

	log.Printf("[kit/db] 开始监听 %v 频道...", channel)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[kit/db] 停止监听通知")
			return

		default:
			ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
			notification, err := conn.Conn().WaitForNotification(ctxWithTimeout)
			cancel()

			if err != nil {
				if err == context.DeadlineExceeded {
					continue
				}
				if err.Error() == "timeout" {
					continue
				}
				if ctx.Err() != nil {
					return
				}
				log.Printf("[kit/db] 等待通知失败: %v", err)
				time.Sleep(1 * time.Second)
				continue
			}

			if notification != nil && onChange != nil {
				onChange(channel, notification.Payload, time.Now())
			}
		}
	}
}
