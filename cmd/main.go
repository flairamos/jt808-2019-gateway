package main

import (
	"context"
	"fmt"
	"jt808-gateway/config"
	"jt808-gateway/internal/server"
	"jt808-gateway/internal/sink"
	redisutil "jt808-gateway/redis_util"
	"jt808-gateway/task"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func main() {
	config.Init()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		config.Config.Database.User,
		config.Config.Database.Passwd,
		config.Config.Database.Addr,
		config.Config.Database.Port,
		config.Config.Database.DBName,
		config.Config.Database.Zone,
	))
	db, err := gorm.Open(dialector, option)

	if err != nil {
		fmt.Printf("连接数据库：%v\n", err)
	}
	config.DB = db

	task.Init()

	// 初始化Redis
	config.RedisConn = redisutil.Init(config.Config.Redis.Host, config.Config.Redis.Password, config.Config.Redis.Database, config.Config.Redis.Port)

	// 初始化批量写入sink
	pgSink := sink.NewPgSink(ctx, db)

	// 启动JT808 TCP服务
	tcpSrv := server.NewTcpServer(ctx, fmt.Sprintf(":%d", config.Config.Server.Port), pgSink)
	if err = tcpSrv.Start(); err != nil {
		panic(err)
	}

	// 优雅退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	fmt.Println("receive shutdown signal")
	cancel()
	tcpSrv.Stop()
}
