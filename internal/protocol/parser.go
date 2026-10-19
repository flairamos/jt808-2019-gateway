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

// assembleLocation 由解析结果组装入库定位结构（各协议版本共用）
// normalize控制手机号是否补零：2019版原样入库，2013/2011版NormalizePhone补齐20位
func assembleLocation(phone string, normalize bool, p *x0200Jt808, createTime string) Location {
	if normalize {
		phone = util.NormalizePhone(phone)
	}
	// 从附加信息中解析举升(ExtraInfoId=0xFC)和闭合状态
	var (
		liftVal  int64
		closeVal int64
	)
	for _, item := range p.ExtraInfo {
		switch strings.ToLower(item.ExtraInfoId) {
		case "fc": // 举升
			liftVal, _ = util.HexToDecimal(item.ExtraInfo)
		case "fd": // 闭合
			closeVal, _ = util.HexToDecimal(item.ExtraInfo)
		}
	}
	return Location{
		Ts:         createTime,
		ImeiNum:    phone,
		Lon:        p.Lon,
		Lat:        p.Lat,
		Altitude:   int16(p.Horizon),
		Speed:      fmt.Sprintf("%d", p.Speed),
		Direction:  fmt.Sprintf("%d", p.Direction),
		CreateTime: time.Now().Format("060102150405"),
		Lift:       int16(liftVal),
		Close:      int16(closeVal),
	}
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
	return assembleLocation(paket.TerminalPhone, false, &paket, ts), nil
}

func GetLocationV2013(message string) (Location, error) {
	var location Location
	paket, err := t0200JT808PaketV2013(message)
	if err != nil {
		return location, err
	}
	ts, err := util.ConvertCodeToTime(paket.TsHex)
	if err != nil {
		return location, errors.New("时间解析失败")
	}
	return assembleLocation(paket.TerminalPhone, true, &paket, ts), nil
}

func GetLocationV2011(message string) (Location, error) {
	var location Location
	paket, err := t0200JT808PaketV2011(message)
	if err != nil {
		return location, err
	}
	ts, err := util.ConvertCodeToTime(paket.TsHex)
	if err != nil {
		return location, errors.New("时间解析失败")
	}
	return assembleLocation(paket.TerminalPhone, true, &paket, ts), nil
}

// 自动识别协议版本并解析0x0200定位上报（2017/2019通用入口）
// 入参为带7e标识位的原始报文，内部先识别版本，再走对应版本的解析流程
func GetLocationAuto(head Jt808Head, message string) (Location, error) {
	// 2019版消息头带协议版本号，2017版head.Version为空
	if head.Version == "" {
		return GetLocationV2013(message)
	}
	return GetLocation(message)
}
