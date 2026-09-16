package redis

import goredis "github.com/go-redis/redis/v8"

// ZAdd 添加元素（score, member）
func ZAdd(key string, members ...*goredis.Z) error {
	return Client.ZAdd(Ctx, key, members...).Err()
}

// ZRange 获取升序范围
func ZRange(key string, start, stop int64) ([]string, error) {
	return Client.ZRange(Ctx, key, start, stop).Result()
}

// ZRevRange 获取降序范围
func ZRevRange(key string, start, stop int64) ([]string, error) {
	return Client.ZRevRange(Ctx, key, start, stop).Result()
}

// ZRangeWithScores 获取带分数的元素
func ZRangeWithScores(key string, start, stop int64) ([]goredis.Z, error) {
	return Client.ZRangeWithScores(Ctx, key, start, stop).Result()
}

// ZScore 获取成员分数
func ZScore(key, member string) (float64, error) {
	return Client.ZScore(Ctx, key, member).Result()
}

// ZRem 删除成员
func ZRem(key string, members ...string) error {
	interfaceMembers := make([]interface{}, len(members))
	for i, member := range members {
		interfaceMembers[i] = member
	}
	return Client.ZRem(Ctx, key, interfaceMembers...).Err()
}

// ZCard 获取有序集合大小
func ZCard(key string) (int64, error) {
	return Client.ZCard(Ctx, key).Result()
}

// ZRank 获取升序排名
func ZRank(key, member string) (int64, error) {
	return Client.ZRank(Ctx, key, member).Result()
}

// ZRevRank 获取降序排名
func ZRevRank(key, member string) (int64, error) {
	return Client.ZRevRank(Ctx, key, member).Result()
}
