package ratelimiter

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"testing"
	"time"
)

func TestSingleRate(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "1234",
		DB:       0,
	})
	rate := NewRateLimiter(client, "ratelimiter_test1").Setting(
		AddLimitConf(10, 2),
		AddLimitConf(5, 1),
	)
	for i := 0; i < 10; i++ {
		fmt.Println(rate.Allow(context.Background()))
		time.Sleep(1 * time.Second)
	}
}

func TestMultiRate(t *testing.T) {
	TestSingleRate(t)
	client := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "1234",
		DB:       0,
	})
	rate := NewMultiRateLimiter(client, []string{"ratelimiter_test1", "ratelimiter_test3"}).Setting(
		AddLimitConf(100, 2),
		AddLimitConf(50, 1),
	)
	for i := 0; i < 3; i++ {
		fmt.Println(rate.AllowMulti(context.Background()))
		time.Sleep(1 * time.Second)
	}
}
