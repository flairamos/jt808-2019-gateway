package task

import (
	"fmt"
	"jt808-gateway/config"
	"jt808-gateway/pkg/util"

	"github.com/gomodule/redigo/redis"
)

func findAllCar() {
	//  方法停掉后删除redis中的key
	defer func() {
		_, _ = config.RedisConn.Do("DEL", config.SetKey)
	}()

	var (
		cars []string
	)
	sql := `
		select imei_num
		from base_transport_vehicle
		where imei_num is not null and imei_num <> ''
	`
	err := config.DB.Raw(sql).Scan(&cars).Error
	if err != nil {
		fmt.Println("查询所有车辆失败", err)
		return
	}

	args := make([]interface{}, 0, len(cars)+1)
	args = append(args, config.SetKey)
	for i := 0; i < len(cars); i++ {
		// 确保与设备上报的手机号格式一致
		cars[i] = util.NormalizePhone(cars[i])
		args = append(args, cars[i])
	}
	_, _ = config.RedisConn.Do("DEL", config.SetKey)
	if len(args) > 1 {
		// only SADD if there are members batchAddCnt
		batchAddCnt, err := redis.Int(config.RedisConn.Do("SADD", args...))
		if err != nil {
			fmt.Println("redis SADD failed:", err)
			return
		}
		fmt.Println("batch added:", batchAddCnt)
	}
}
