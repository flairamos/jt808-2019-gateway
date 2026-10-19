package main

import (
	"context"
	"fmt"
	"jt808-gateway/config"
	"jt808-gateway/internal/server"
	"jt808-gateway/internal/sink"
	"jt808-gateway/pkg/db"
	"jt808-gateway/pkg/redis_util"
	"jt808-gateway/task"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	config.Init()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 初始化Redis
	redisutil.Init(ctx, config.Config.Redis.Host, config.Config.Redis.Password, config.Config.Redis.Database, config.Config.Redis.Port)
	// db
	db.Init(config.Config.Database.User, config.Config.Database.Passwd, config.Config.Database.Addr, config.Config.Database.Port, config.Config.Database.DBName, config.Config.Database.Zone)

	task.Init()

	// 初始化批量写入sink
	pgSink := sink.NewPgSink(ctx, db.DB)

	// 启动JT808 TCP服务
	tcpSrv := server.NewTcpServer(ctx, fmt.Sprintf(":%d", config.Config.Server.Port), pgSink)
	if err := tcpSrv.Start(); err != nil {
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
