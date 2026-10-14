package config

import (
	"github.com/gomodule/redigo/redis"
	"gorm.io/gorm"
)

// DB 数据库全局连接
var DB *gorm.DB

var RedisConn redis.Conn

const SetKey = "imei_whitelist"
