package db

import (
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"time"
)

// DB 数据库全局连接
var DB *gorm.DB

func Init(user, pass, host string, port int, name, zone string) {
	// 初始化数据库
	option := &gorm.Config{
		// 配置查询时表非复数形式
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		DefaultContextTimeout: 30 * time.Second,
	}
	dialector := postgres.Open(fmt.Sprintf(
		"user=%s password=%s host=%s port=%d dbname=%s TimeZone=%s",
		user,
		pass,
		host,
		port,
		name,
		zone,
	))
	db, err := gorm.Open(dialector, option)
	if err != nil {
		fmt.Printf("连接数据库：%v\n", err)
		panic(err)
	}
	DB = db
	fmt.Println("db init success")
}
