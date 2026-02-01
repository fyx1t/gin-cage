package gincage

// Bucket implementations shouldnt use any sync primitives to
// sync underlying core actions because it can help only if
// package is used in one process.
//
// For cases when we have something like load balancer we should use
// underlying sync mechanisms

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

var (
	// Default tokens cap
	MaxTokensCapDefault = 10
	// Default time for new tokens append
	NewTokenAppendTimeDefault = time.Duration(10 * time.Second)
	// Default duration
	DurationDefault = time.Duration(30 * time.Minute)
)

type SyncUpdate struct {
	Object    string
	Tokens    int
	Timestamp time.Time
}

// Bucket: simple collection of ips with their awailable tokens.
//
// Should be implemented by real storage under the hood.
type Bucket interface {
	// Try to get token and walk through.
	// If no tokens awailable or error occured while connecting to bucket, returns (false, error).
	// Otherwise returns (true, nil).
	Walk(ctx *gin.Context) error

	// Closes connection to bucket
	Close() error
}

type BucketConfigs struct {
	// Bucket host
	Host string
	// Bucket port
	Port int
	// Bucket network (tcp, udp). Omit empty for tcp
	Network string

	// Max count of tokens. If <= 0, uses MaxTokensCapDefault
	Capability int
	// Time after object will expire. If <= 0, uses DurationDefault
	Duration time.Duration
	// Time after new tokens append. If <= 0, uses NewTokenAppendDefault
	NewTokenAppendTime time.Duration
}

type RedisBucket struct {
	core *redis.Client

	cap             int
	dur             time.Duration
	tokenAppendTime time.Duration
}

// Implements Bucket interface and allows to use redis as tokens bucket.
//
// Allows to use existing redis connection
func NewRedisBucketWithClient(cfg BucketConfigs, c *redis.Client) Bucket {
	if cfg.Capability <= 0 {
		cfg.Capability = MaxTokensCapDefault
	}

	if cfg.Duration <= 0 {
		cfg.Duration = DurationDefault
	}

	if cfg.NewTokenAppendTime <= 0 {
		cfg.NewTokenAppendTime = NewTokenAppendTimeDefault
	}

	return &RedisBucket{
		core:            c,
		cap:             cfg.Capability,
		dur:             cfg.Duration,
		tokenAppendTime: cfg.NewTokenAppendTime,
	}
}

// Implements Bucket interface and allows to use redis as tokens bucket.
//
// Creates new redis client and returns error if it was broken
func NewRedisBucket(cfg BucketConfigs) (Bucket, error) {
	if cfg.Network == "" {
		cfg.Network = "tcp"
	}
	c := redis.NewClient(&redis.Options{
		Network: cfg.Network,
		Addr:    net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
	})
	if err := c.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}

	if cfg.Capability <= 0 {
		cfg.Capability = MaxTokensCapDefault
	}

	if cfg.Duration <= 0 {
		cfg.Duration = DurationDefault
	}

	if cfg.NewTokenAppendTime <= 0 {
		cfg.NewTokenAppendTime = NewTokenAppendTimeDefault
	}

	return &RedisBucket{
		core:            c,
		cap:             cfg.Capability,
		dur:             cfg.Duration,
		tokenAppendTime: cfg.NewTokenAppendTime,
	}, nil
}

// Closes connection to redis
func (b RedisBucket) Close() error {
	return b.core.Close()
}

// Try to get token and walk through.
// If no tokens awailable or error occured while connecting to redis, returns (false, error).
// Otherwise returns (true, nil).
func (b RedisBucket) Walk(ctx *gin.Context) error {
	if b.core == nil {
		return errors.New("redis core is nil")
	}

	ip := ctx.ClientIP()
	var tokens int
	var t time.Time
	for {
		err := b.core.Watch(ctx, func(tx *redis.Tx) error {
			r, err := tx.Get(ctx, "ginratelimiter:"+ip).Result()
			if err != nil {
				if !errors.Is(err, redis.Nil) {
					return err
				}
				tokens = b.cap
				t = time.Now()
			} else {
				d := strings.Split(r, "|")
				if len(d) != 2 {
					return ErrBadSyntaxInStorage
				}
				tokens, err = strconv.Atoi(d[0])
				if err != nil {
					return err
				}

				t, err = time.Parse(time.RFC3339, d[1])
				if err != nil {
					return err
				}

				// if we can append tokens
				if tokens < b.cap {
					p := time.Since(t)
					// if we can append tokens right now
					if p >= b.tokenAppendTime {
						// check how many tokens we can add to bucket
						add := int(p / b.tokenAppendTime)

						// get number of tokens we can add to bucket under cap
						add = min(add, b.cap-tokens)

						// add tokens
						tokens += add

						// time shift
						//
						// we try to leave extra time when we have it,
						// but also avoid situations where there is too much time left
						// when we fulfill tokens.
						if tokens == b.cap {
							t = time.Now()
						} else {
							t = t.Add(time.Duration(add) * b.tokenAppendTime)
						}

					}
				}

				if tokens <= 0 {
					return ErrNoTokensAwailable
				}
			}

			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				return pipe.Set(ctx, "ginratelimiter:"+ip, strconv.Itoa(tokens-1)+"|"+t.Format(time.RFC3339), b.dur).Err()
			})

			return err
		}, "ginratelimiter:"+ip)

		if err != nil {
			if !errors.Is(err, redis.TxFailedErr) {
				return err
			}
			continue
		}
		break
	}

	return nil
}
