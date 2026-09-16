package redis

// ListLPush 从左侧插入
func ListLPush(key string, values ...interface{}) error {
	return Client.LPush(Ctx, key, values...).Err()
}

// ListRPush 从右侧插入
func ListRPush(key string, values ...interface{}) error {
	return Client.RPush(Ctx, key, values...).Err()
}

// ListLPop 从左侧弹出
func ListLPop(key string) (string, error) {
	return Client.LPop(Ctx, key).Result()
}

// ListRPop 从右侧弹出
func ListRPop(key string) (string, error) {
	return Client.RPop(Ctx, key).Result()
}

// ListRange 获取范围 [start, stop]
func ListRange(key string, start, stop int64) ([]string, error) {
	return Client.LRange(Ctx, key, start, stop).Result()
}

// ListLen 获取长度
func ListLen(key string) (int64, error) {
	return Client.LLen(Ctx, key).Result()
}

// ListDel 删除整个 list
func ListDel(key string) error {
	return Client.Del(Ctx, key).Err()
}
