package ratelimiter

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"time"
)

var limitScript = `
local keys = KEYS
local strategies = ARGV
local currentScore = tonumber(strategies[#strategies]) -- 当前时间戳

-- 从 strategies 中移除当前时间戳
table.remove(strategies)

local results = {}
for i = 1, #keys do
    local key = keys[i]
	local pass = true
	for j = 1, #strategies, 2 do
    	local windowSize = tonumber(strategies[j])
    	local maxRequests = tonumber(strategies[j + 1])
    	local windowEnd = currentScore - windowSize
    	local count = redis.call('ZCOUNT', key, windowEnd, '+inf')
    	if count >= maxRequests then
			pass = false
			break
    	end
	end
	if pass == true then
		table.insert(results, 1)
		redis.call('ZADD', key, currentScore, 'request:' .. currentScore)
	else 
		table.insert(results, 0)
	end
end

return results
`

var clearScript = `
local keys = KEYS
local expireTime = tonumber(ARGV[1])
local curTime = tonumber(ARGV[2])
for i = 1, #keys do
	local key = keys[i]
	redis.call('EXPIRE', key, expireTime)
	redis.call('ZREMRANGEBYSCORE', key, '-inf', curTime - expireTime - 1)
end
`

type StrategyConf struct {
	n int64 //每n秒
	x int64 //最多x次
}

type RateLimiter struct {
	client     *redis.Client
	key        []string
	expiration int64
	strategies []*StrategyConf
}

func NewRateLimiter(client *redis.Client, key string) *RateLimiter {
	return &RateLimiter{
		client: client,
		key:    []string{key},
	}
}

func NewMultiRateLimiter(client *redis.Client, key []string) *RateLimiter {
	return &RateLimiter{
		client: client,
		key:    key,
	}
}

func SetExpiration(duration int64) func(r *RateLimiter) {
	return func(r *RateLimiter) {
		r.expiration = duration
	}
}

func AddLimitConf(n, x int64) func(r *RateLimiter) {
	return func(r *RateLimiter) {
		r.strategies = append(r.strategies, &StrategyConf{
			n: n,
			x: x,
		})
	}
}

func (r *RateLimiter) Setting(opt ...func(r *RateLimiter)) *RateLimiter {
	for _, o := range opt {
		o(r)
	}
	return r
}

func (r *RateLimiter) Allow(ctx context.Context) (bool, error) {
	rs, err := r.AllowMulti(ctx)
	if err != nil {
		return false, err
	}
	return rs[r.key[0]], nil
}

func (r *RateLimiter) AllowMulti(ctx context.Context) (map[string]bool, error) {
	rs := make(map[string]bool, len(r.key))
	if len(r.strategies) == 0 {
		for _, key := range r.key {
			rs[key] = true
		}
		return rs, nil
	}

	strategies := make([]interface{}, 0, len(r.strategies)*2+1)
	// 计算最长超时时间，默认以频控配置最长的时间为准，一般这就够用
	var maxExp int64
	if r.expiration > 0 {
		maxExp = r.expiration
	} else {
		// 整理传给lua的args，变成一维数组，n,x,n,x这样结构
		for _, c := range r.strategies {
			strategies = append(strategies, c.n, c.x)
			if c.n > maxExp {
				maxExp = c.n
			}
		}
	}
	// 调用Lua脚本
	strategies = append(strategies, time.Now().Unix())
	result, err := r.client.Eval(ctx, limitScript, r.key, strategies...).Result()
	if err != nil {
		return rs, err
	}
	formatRs, ok := result.([]interface{})
	if !ok {
		return rs, fmt.Errorf("redis result type wrong")
	}
	needClearKey := make([]string, 0, len(r.key))
	for i, key := range r.key {
		allow := formatRs[i].(int64) == 1
		rs[key] = allow
		if allow {
			needClearKey = append(needClearKey, key)
		}
	}
	//清掉过期的key并给key一个过期时间，防止key遗留过久
	go func(ctx context.Context) {
		if len(needClearKey) == 0 {
			return
		}
		_, _ = r.client.Eval(ctx, clearScript, needClearKey, maxExp, time.Now().Unix()).Result()
	}(context.WithoutCancel(ctx))
	return rs, nil
}
