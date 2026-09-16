package redis

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

const (
	// FindTableKeyPrefix 通用表查询 Redis 缓存前缀（勿与 token: 混淆）
	FindTableKeyPrefix = "ft:"
	// FindTableTTLBase 表缓存基础过期时间
	FindTableTTLBase = 10 * time.Minute
	// FindTableTTLJitter 过期抖动上限，降低齐步失效
	FindTableTTLJitter = 2 * time.Minute
)

// FindTableTTL 返回带抖动的表缓存 TTL
func FindTableTTL() time.Duration {
	jitter := time.Duration(0)
	if FindTableTTLJitter > 0 {
		jitter = time.Duration(rand.Int63n(int64(FindTableTTLJitter)))
	}
	return FindTableTTLBase + jitter
}

// FindTableAllKey 全表缓存 key
func FindTableAllKey(tableName string) string {
	return FindTableKeyPrefix + tableName + ":all"
}

// FindTableItemKey 按字段缓存 key
func FindTableItemKey(tableName, field, escapedValue string) string {
	return fmt.Sprintf("%s%s:item:%s:%s", FindTableKeyPrefix, tableName, field, escapedValue)
}

// FindTablePageKey 分页缓存 key
func FindTablePageKey(tableName string, page, pageSize int) string {
	return fmt.Sprintf("%s%s:page:%d:%d", FindTableKeyPrefix, tableName, page, pageSize)
}

// ScanDeleteByPrefix 按 MATCH 模式删除 key（SCAN，避免 KEYS 阻塞）。
// pattern 示例：ft:users:* 、findTableNameHandler_users*
// 不会匹配 token: 会话（调用方勿传过于宽泛的 *）。
func ScanDeleteByPrefix(pattern string) (int, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || Client == nil {
		return 0, nil
	}
	var cursor uint64
	deleted := 0
	for {
		keys, next, err := Client.Scan(Ctx, cursor, pattern, 100).Result()
		if err != nil {
			return deleted, err
		}
		if len(keys) > 0 {
			n, err := Client.Del(Ctx, keys...).Result()
			if err != nil {
				return deleted, err
			}
			deleted += int(n)
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return deleted, nil
}

// ClearFindTableCacheForTable 删除某表相关的新/旧前缀缓存
func ClearFindTableCacheForTable(tableName string) (int, error) {
	tableName = strings.TrimSpace(tableName)
	if tableName == "" {
		return 0, nil
	}
	total := 0
	patterns := []string{
		FindTableKeyPrefix + tableName + ":*",
		"findTableNameHandler_" + tableName + "*",
		"findTable_" + tableName + "_*",
	}
	for _, p := range patterns {
		n, err := ScanDeleteByPrefix(p)
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// ClearAllFindTableCaches 清理全部通用表缓存（不含 token:*）
func ClearAllFindTableCaches() (int, error) {
	total := 0
	patterns := []string{
		FindTableKeyPrefix + "*",
		"findTableNameHandler_*",
		"findTable_*",
	}
	for _, p := range patterns {
		n, err := ScanDeleteByPrefix(p)
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
