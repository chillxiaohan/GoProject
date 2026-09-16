package redis

// HashSet 设置 hash 字段
func HashSet(key, field, value string) error {
	return Client.HSet(Ctx, key, field, value).Err()
}

// HashMSet 批量设置 hash
func HashMSet(key string, fields map[string]interface{}) error {
	return Client.HMSet(Ctx, key, fields).Err()
}

// HashGet 获取 hash 字段
func HashGet(key, field string) (string, error) {
	return Client.HGet(Ctx, key, field).Result()
}

// HashGetAll 获取所有字段
func HashGetAll(key string) (map[string]string, error) {
	return Client.HGetAll(Ctx, key).Result()
}

// HashDel 删除字段
func HashDel(key string, fields ...string) error {
	return Client.HDel(Ctx, key, fields...).Err()
}

// HashExists 判断字段是否存在
func HashExists(key, field string) (bool, error) {
	exists, err := Client.HExists(Ctx, key, field).Result()
	return exists, err
}
