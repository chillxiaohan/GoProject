package redis

// SetAdd 添加元素
func SetAdd(key string, members ...interface{}) error {
	return Client.SAdd(Ctx, key, members...).Err()
}

// SetMembers 获取所有元素
func SetMembers(key string) ([]string, error) {
	return Client.SMembers(Ctx, key).Result()
}

// SetIsMember 判断是否是成员
func SetIsMember(key string, member interface{}) (bool, error) {
	exists, err := Client.SIsMember(Ctx, key, member).Result()
	return exists, err
}

// SetRem 删除元素
func SetRem(key string, members ...interface{}) error {
	return Client.SRem(Ctx, key, members...).Err()
}

// SetCard 获取集合大小
func SetCard(key string) (int64, error) {
	return Client.SCard(Ctx, key).Result()
}

// SetPop 随机弹出一个元素
func SetPop(key string) (string, error) {
	return Client.SPop(Ctx, key).Result()
}
