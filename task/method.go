package task

import (
	"fmt"
	"jt808-gateway/pkg/db"
	redisutil "jt808-gateway/pkg/redis_util"
	"jt808-gateway/pkg/util"
	"jt808-gateway/pkg/variables"

	"github.com/gomodule/redigo/redis"
)

func findAllCar() {
	//  方法停掉后删除redis中的key
	defer func() {
		// redisutil.RedisConn.Do("DEL", variables.SetKey)
	}()

	var (
		cars []string
	)
	sql := `
		select imei_num
		from base_transport_vehicle
		where imei_num is not null and imei_num <> ''
	`
	err := db.DB.Raw(sql).Scan(&cars).Error
	if err != nil {
		fmt.Println("查询所有车辆失败", err)
		return
	}

	args := make([]interface{}, 0, len(cars)+1)
	args = append(args, variables.SetKey)
	for i := 0; i < len(cars); i++ {
		// 确保与设备上报的手机号格式一致
		cars[i] = util.NormalizePhone(cars[i])
		args = append(args, cars[i])
	}
	_, _ = redisutil.RedisConn.Do("DEL", variables.SetKey)
	if len(args) > 1 {
		// only SADD if there are members batchAddCnt
		batchAddCnt, err := redis.Int(redisutil.RedisConn.Do("SADD", args...))
		if err != nil {
			fmt.Println("redis SADD failed:", err)
			return
		}
		fmt.Println("batch added:", batchAddCnt)
	}
}
