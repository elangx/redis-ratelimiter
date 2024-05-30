# redis-ratelimiter

redis-ratelimiter helps you use redis sorted set to make a sliding window limiter.It supports multi limit setting, like 5 time per sec & 10 time per hour. I use it for push limit. It also can check multi keys in one request.

Examples can be seen in the test file.