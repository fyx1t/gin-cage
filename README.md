# gin-cage - simple ip rate limiter realization for gin framework

### Current storage ports:
- redis

### Basic usage (redis):
```Go
router := gin.New(opts...)
bucket, err := gincage.NewRedisBucket(gincage.BucketConfigs{
    ...
})
if err != nil {
	return nil
}
limiter := gincage.NewLimiter(
	...
)
router.Use(limiter.WalkThrough())
```
### Reuse existing redis client:
```Go
redisClient := redis.NewClient(&redis.Options{
	...
})
// use redisClient for you own needs
...
// and reuse it with gincage
bucket := gincage.NewRedisBucketWithClient(gincage.BucketConfigs{
    ...
}, redisClient)
...
