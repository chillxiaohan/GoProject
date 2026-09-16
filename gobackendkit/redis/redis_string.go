package redis

import "time"

// StringSet 设置字符串
func StringSet(key, value string, expiration time.Duration) error {
	return Client.Set(Ctx, key, value, expiration).Err()
}

// StringGet 获取字符串
func StringGet(key string) (string, error) {
	return Client.Get(Ctx, key).Result()
}

// StringDel 删除字符串
func StringDel(key string) error {
	return Client.Del(Ctx, key).Err()
}

// StringExpire 设置过期时间
func StringExpire(key string, expiration time.Duration) error {
	return Client.Expire(Ctx, key, expiration).Err()
}

// StringExists 检查是否存在
func StringExists(key string) (bool, error) {
	count, err := Client.Exists(Ctx, key).Result()
	return count > 0, err
}
