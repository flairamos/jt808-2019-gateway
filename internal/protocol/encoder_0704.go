package protocol

import (
	"errors"
	"fmt"
	"jt808-gateway/pkg/util"
)

// # JT808 0x0704 批量定位数据上报解析
//
// 消息体格式（2013与2019一致）：
//
//	位置数据项个数 WORD(2B) + 位置上报类型 BYTE(1B，0=正常批量 1=补报)
//	+ N × [ 数据项长度 WORD(2B) + 0x0200消息体（无消息头、无校验码，含附加信息TLV） ]
//
// 数据项归属：0200体内无终端手机号，统一归属消息头中的终端手机号（即上报终端自身）。
// 实现说明：逐项按长度前缀遍历到消息体末尾，声明个数仅用于结果核对——
// 个别聚合设备声明的个数与实际不符（实测有声明0x0101实际1项的），弹性遍历可兼容。

// GetLocationBatchAuto 自动识别协议版本并解析0x0704批量定位上报
// 入参为带7e标识位的完整原始帧（与GetLocationAuto一致），返回每项定位（按消息头版本应用手机号补零规则）
func GetLocationBatchAuto(head Jt808Head, message string) ([]Location, error) {
	// 消息体起点：2019版消息头34个hex字符，2013/2011版24个
	bodyStart := jt808HeadLenV2017
	if head.Version == "1" {
		bodyStart = t0200HeadLenV2019
	}
	// 至少需要：个数(4) + 类型(2) + 一个数据项的长度前缀(4)，另加末尾2位校验码
	if len(message) < bodyStart+12 {
		return nil, fmt.Errorf("0704消息体长度不足，至少需要%d个字符，实际%d个", bodyStart+12, len(message))
	}
	body := message[bodyStart : len(message)-2] // 去除校验码

	declared, err := util.HexToDecimal(body[0:4])
	if err != nil {
		return nil, errors.New("0704数据项个数解析失败")
	}
	// body[4:6]为位置上报类型（0=正常批量 1=补报），当前入库逻辑相同，不做区分

	var locs []Location
	offset := 6
	skipped := 0
	for offset+4 <= len(body) {
		itemLen64, err := util.HexToDecimal(body[offset : offset+4])
		if err != nil {
			break
		}
		itemLen := int(itemLen64) * 2 // 每字节2个hex字符
		// 数据项至少28字节基本信息；长度超出剩余报文按截断处理，结束遍历
		if itemLen < 56 || offset+4+itemLen > len(body) {
			break
		}
		b, err := parseT0200Body(body[offset+4 : offset+4+itemLen])
		if err != nil {
			skipped++
			fmt.Printf("0704数据项解析失败(偏移%d): %v\n", offset, err)
		} else if ts, terr := util.ConvertCodeToTime(b.TsHex); terr != nil {
			skipped++
			fmt.Printf("0704数据项时间解析失败(偏移%d): %v\n", offset, terr)
		} else {
			// 数据项归属上报终端自身；2019版手机号原样，2013/2011版补零
			locs = append(locs, assembleLocation(head.TerminalPhone, head.Version != "1", &b, ts))
		}
		offset += 4 + itemLen
	}

	if declared > 0 && int64(len(locs)) != declared {
		// 声明个数与实际不符：能解析的照常入库，但留痕便于排查设备侧问题
		fmt.Printf("0704数据项个数与实际不符: 声明%d 跳过%d 实际%d (终端%s)\n",
			declared, skipped, len(locs), head.TerminalPhone)
	}
	if len(locs) == 0 && declared > 0 {
		return nil, errors.New("0704批量定位无可解析数据项")
	}
	return locs, nil
}
