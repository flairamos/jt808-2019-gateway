package redisutil

import (
	"context"
	"errors"
	"fmt"
	"jt808-gateway/pkg/variables"

	"github.com/gomodule/redigo/redis"
)

var RedisConn redis.Conn

func Init(ctx context.Context, host, pass string, database, port int) {
	conn, err := redis.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		fmt.Printf("redis dial error: %v\n", err)
		panic(err)
	}

	if pass != "" {
		_, err = conn.Do("auth", pass)
		if err != nil {
			panic(errors.New("redis auth failed"))
		}
	}
	RedisConn = conn
	// 选择第一个数据库
	replay, err := conn.Do("SELECT", database)
	if err != nil {
		_ = replay
		panic(errors.New("redis select database failed"))
	}
	fmt.Println("redis load success")
	go stop(ctx, conn)
	return
}

func stop(ctx context.Context, conn redis.Conn) {
	<-ctx.Done() // 等信号
	_, _ = conn.Do("DEL", variables.SetKey)
	_ = conn.Close()
	fmt.Println("redis cleanup done") // 日志放这里
}
