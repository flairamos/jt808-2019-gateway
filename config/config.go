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
		fmt.Println(err)
		return
	}
	err = yaml.Unmarshal(data, &Config)
	if err != nil {
		fmt.Println(err)
		return
	}
}
