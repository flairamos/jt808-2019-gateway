package redisutil

import (
	"errors"
	"fmt"

	"github.com/gomodule/redigo/redis"
)

func Init(host, pass string, database, port int) redis.Conn {
	conn, err := redis.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		panic(errors.New("conn redis failed"))
	}
	if pass != "" {
		_, err = conn.Do("auth", pass)
		if err != nil {
			panic(errors.New("redis auth failed"))
		}
	}
	// 选择第一个数据库
	_, err = conn.Do("SELECT", database)
	if err != nil {
		panic(errors.New("redis select database failed"))
	}

	return conn
}
