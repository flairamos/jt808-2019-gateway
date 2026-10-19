package task

import (
	"github.com/robfig/cron/v3"
	"log"
)

func Init() {
	c := cron.New()
	task1(c)
	c.Start()
}

func task1(c *cron.Cron) {
	// 每隔10分钟执行一次，删除过期的定位信息
	_, err := c.AddFunc("*/10 * * * *", findAllCar)
	if err != nil {
		log.Println("添加定时任务失败1")
		return
	}
	findAllCar()
}
