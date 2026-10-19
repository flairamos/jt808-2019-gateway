package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// # 配置管理、超时、缓冲区参数
var Config config

type config struct {
	Server   server    `yaml:"server"`
	Database database  `yaml:"database"`
	Redis    redisConf `yaml:"redis"`
}

type server struct {
	Port int `yaml:"port"`
}

type database struct {
	Driver string `yaml:"driver"`
	User   string `yaml:"user"`
	Passwd string `yaml:"passwd"`
	Addr   string `yaml:"addr"`
	Port   int    `yaml:"port"`
	DBName string `yaml:"dbName"`
	Zone   string `yaml:"zone"`
}

type redisConf struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	Database int    `yaml:"database"`
}

func Init() {
	data, err := os.ReadFile("./config.yaml")
	if err != nil {
		// 配置读不到必须直接终止：否则Config全零值，服务会监听到随机端口(0)且无库可写
		panic(fmt.Sprintf("读取配置文件config.yaml失败: %v", err))
	}
	err = yaml.Unmarshal(data, &Config)
	if err != nil {
		panic(fmt.Sprintf("解析配置文件config.yaml失败: %v", err))
	}
}
