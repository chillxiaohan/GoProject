package redis

import (
	"context"
	"time"

	goredis "github.com/go-redis/redis/v8"
)

var (
	Client *goredis.Client
	Ctx    = context.Background()
)

// InitRedis 初始化 Redis 客户端
func InitRedis(addr, password string, db int) error {
	Client = goredis.NewClient(&goredis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		PoolSize:     10,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
	_, err := Client.Ping(Ctx).Result()
	return err
}

// Close 关闭 Redis 连接
func Close() {
	if Client != nil {
		_ = Client.Close()
	}
}
