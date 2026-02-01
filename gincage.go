// Simple ip rate limiter realization for gin framework
//
// Current storage ports:
//
// - redis
//
// Basic usage (redis):
//
//	 router := gin.New(opts...)
//	 bucket, err := gincage.NewRedisBucket(gincage.BucketConfigs{
//		 ...
//	 })
//	 if err != nil {
//		 return nil
//	 }
//	 limiter := gincage.NewLimiter(
//		 ...
//	 )
//	 router.Use(limiter.WalkThrough())
//
// Reuse existing redis client:
//
//	 redisClient := redis.NewClient(&redis.Options{
//		 ...
//	 })
//	 // use redisClient for you own needs
//	 ...
//	 // and reuse it with gincage
//	 bucket := gincage.NewRedisBucketWithClient(gincage.BucketConfigs{
//		...
//	}, redisClient)
//
// ...
package gincage

import (
	"context"
	"errors"
	"io"

	"github.com/gin-gonic/gin"
)

var (
	DefaultServerError = struct {
		Error string `json:"error"`
	}{
		Error: "server error occured",
	}
	DefaultTooManyRequestsError = struct {
		Error string `json:"error"`
	}{
		Error: "too many requests, try again later",
	}
)

type limiter struct {
	bucket               Bucket
	logger               io.Writer
	serverError          any
	tooManyRequestsError any
}

func NewLimiter(ctx context.Context, bucket Bucket, logger io.Writer, serverError, tooManyRequestsError any) limiter {
	return limiter{
		bucket:               bucket,
		logger:               logger,
		serverError:          serverError,
		tooManyRequestsError: tooManyRequestsError,
	}
}

// Returns HTTP 429 Too Many Requests if rate was limited
func (l limiter) WalkThrough() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if err := l.bucket.Walk(ctx); err != nil {
			if errors.Is(err, ErrNoTokensAwailable) {
				ctx.AbortWithStatusJSON(429, l.tooManyRequestsError)
				return
			}
			l.logger.Write([]byte(err.Error()))
			ctx.AbortWithStatusJSON(500, l.serverError)
		}
	}
}
