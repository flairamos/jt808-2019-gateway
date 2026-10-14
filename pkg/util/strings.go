package util

import (
	"errors"
	"fmt"
)

// NormalizePhone 规范化终端手机号/IMEI：
// JT808标准中终端手机号为BCD编码，规范长度10字节(20位hex)，但部分设备只上报6字节(12位)
// 或在前部补0，例如 00000000013925080356 与 013925080356 实际是同一个号码 13925080356。
// 统一填充前导0，保证设备上报值与数据库配置值可以匹配。
func NormalizePhone(phone string) string {
	res := fmt.Sprintf("%020s", phone)
	return res
}

// 250521094621 -> YYYY-MM-DD hh:mm:ss
func ConvertCodeToTime(ts string) (string, error) {
	if len(ts) != 12 {
		return "", errors.New("字符串必须是12位数")
	}
	year := ts[0:2]
	month := ts[2:4]
	day := ts[4:6]
	hour := ts[6:8]
	minute := ts[8:10]
	second := ts[10:12]

	// 拼接规则
	timeStr := fmt.Sprintf("20%s-%s-%s %s:%s:%s", year, month, day, hour, minute, second)
	return timeStr, nil
}
