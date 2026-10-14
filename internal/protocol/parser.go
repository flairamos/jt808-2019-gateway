package protocol

import (
	"errors"
	"fmt"
	"jt808-gateway/pkg/util"
	"strings"
	"time"
)

// # 各类消息解析（0x0200 定位上报重点）
type Location struct {
	Ts         string  `gorm:"column:ts;comment:数据入库时间" json:"ts"`                           // 数据入库时间
	ImeiNum    string  `gorm:"column:imei_num;comment:定位装置IMEI号" json:"imei_num"`            // 定位装置IMEI号
	Lon        float64 `gorm:"column:lon;comment:GCJ02经度" json:"lon"`                        // GCJ02经度
	Lat        float64 `gorm:"column:lat;comment:纬度" json:"lat"`                             // 纬度
	Altitude   int16   `gorm:"column:altitude;comment:海拔" json:"altitude"`                   // 海拔
	Speed      string  `gorm:"column:speed;comment:速度" json:"speed"`                         // 速度
	Direction  string  `gorm:"column:direction;comment:方向，0-359，正北为 0，顺时针" json:"direction"` // 方向，0-359，正北为 0，顺时针
	CreateTime string  `gorm:"column:create_time;comment:数据创建时间" json:"create_time"`         // 数据创建时间
	Lift       int16   `gorm:"column:lift;comment:举升" json:"lift"`                           // 举升
	Close      int16   `gorm:"column:close;comment:闭合" json:"close"`
}

func GetLocation(message string) (Location, error) {
	var location Location
	paket, err := t0200JT808Paket(message)
	if err != nil {
		return location, err
	}
	ts, err := util.ConvertCodeToTime(paket.TsHex)
	if err != nil {
		return location, errors.New("时间解析失败")
	}
	// 从附加信息中解析举升(ExtraInfoId=0xFC)和闭合状态
	var (
		liftVal  int64
		closeVal int64
	)
	for _, item := range paket.ExtraInfo {
		switch strings.ToLower(item.ExtraInfoId) {
		case "fc": // 举升
			liftVal, _ = util.HexToDecimal(item.ExtraInfo)
		case "fd": // 闭合
			closeVal, _ = util.HexToDecimal(item.ExtraInfo)
		}
	}
	location = Location{
		Ts:         time.Now().Format("2006-01-02 15:04:05"),
		ImeiNum:    paket.TerminalPhone,
		Lon:        paket.Lon,
		Lat:        paket.Lat,
		Altitude:   int16(paket.Horizon),
		Speed:      fmt.Sprintf("%d", paket.Speed),
		Direction:  fmt.Sprintf("%d", paket.Direction),
		CreateTime: ts,
		Lift:       int16(liftVal),
		Close:      int16(closeVal),
	}
	return location, nil
}
