package protocol

import (
	"errors"
	"fmt"
	"jt808-gateway/pkg/util"
	"strings"
)

// # 平台下行消息编码（0x8100、下发指令）

type x0200Jt808 struct {
	MessageId     string          // 消息ID
	ProperBinary  string          // 消息体属性对象二进制表示
	Length        int             // 消息属性-消息体长度
	Version       string          // // 消息属性-消息版本
	TerminalPhone string          // 终端手机号
	StreamNum     int64           // 消息流水号-10进制
	StreamId      string          // 消息流水号
	AlarmSign     string          // 报警标识
	StatusSign    string          // 状态标识
	StatusBinary  string          // 状态标识-二进制位
	Lat           float64         // 纬度
	Lon           float64         // 经度
	Horizon       int64           // 海拔
	Speed         int64           // 速度
	Direction     int64           // 方向
	TsHex         string          // 时间
	ExtraInfo     []extraInfoItem // 额外信息

}

type jt808Head struct {
	MessageId     string // 消息ID
	ProperBinary  string // 消息体属性对象二进制表示
	Length        int    // 消息属性-消息体长度
	Version       string // // 消息属性-消息版本
	TerminalPhone string // 终端手机号
	StreamNum     int64  // 消息流水号-10进制
	StreamId      string // 消息流水号
}

type extraInfoItem struct {
	ExtraInfoId     string
	ExtraInfoLength string
	ExtraInfo       string
}

// 检查标识位置（报文以7e开头和结尾，可能带有0x前缀）
func checkSign(message string) bool {
	data := strings.ToLower(strings.TrimSpace(message))
	data = strings.TrimPrefix(data, "0x")
	return strings.HasPrefix(data, "7e") && strings.HasSuffix(data, "7e")
}

func transferMessage(message string) string {
	// 去除首尾空白字符（如\r\n）和所有空格
	message = strings.TrimSpace(message)
	message = strings.ReplaceAll(message, " ", "")
	// 部分设备会发送0x/0X前缀，去除
	if len(message) >= 2 && (message[:2] == "0x" || message[:2] == "0X") {
		message = message[2:]
	}
	// 剥离7e标识符
	message = message[2 : len(message)-2]
	message = strings.ReplaceAll(message, "7d01", "7d")
	message = strings.ReplaceAll(message, "7d02", "7e")
	return message
}

// 根据消息ID判断报文类型
func ParsePaketMessageId(message string) (jt808Head, string, error) {
	var head jt808Head
	sign := checkSign(message)
	if !sign {
		return head, "", errors.New("报文内容或者格式不对！")
	}
	message = transferMessage(message)
	messageId := message[:4]
	// 解析消息体属性
	property := message[4:8]
	propertyBinary, err := util.HexToBinary(property)
	if err != nil {
		return head, "", errors.New("报文长度解析失败")
	}
	// 小于16位自动补0
	if len(propertyBinary) < 16 {
		for i := len(propertyBinary); i < 16; i++ {
			propertyBinary = "0" + propertyBinary
		}
	}
	length, err := util.BinaryToDecimal(propertyBinary[6:])
	if err != nil {
		return head, "", errors.New("报文长度解析失败")
	}
	// 解析协议版本号
	version := message[8:10]
	// 解析终端手机号
	terminalPhone := message[10:30]
	// 解析消息流水号
	streamId := message[30:34]
	streamNum, err := util.HexToDecimal(streamId)
	if err != nil {
		return head, "", errors.New("消息流水号解析失败")
	}

	head = jt808Head{
		MessageId:     messageId,
		ProperBinary:  propertyBinary,
		Length:        length,
		Version:       version,
		TerminalPhone: terminalPhone,
		StreamNum:     streamNum,
		StreamId:      streamId,
	}
	return head, message, nil
}

// 这里只做定位信息报文解析
// 去除后标识符和转义后的报文
func t0200JT808Paket(message string) (x0200Jt808, error) {
	var location x0200Jt808
	// 校验报文最小长度：消息头(34) + 报警(8) + 状态(8) + 纬度(8) + 经度(8) + 高程(4) + 速度(4) + 方向(4) + 时间(12) = 90
	if len(message) < 90 {
		return location, fmt.Errorf("报文长度不足，至少需要90个字符，实际%d个", len(message))
	}
	messageId := message[:4]
	// 解析消息体属性
	property := message[4:8]
	propertyBinary, err := util.HexToBinary(property)
	if err != nil {
		return location, errors.New("报文长度解析失败")
	}
	// 小于16位自动补0
	if len(propertyBinary) < 16 {
		for i := len(propertyBinary); i < 16; i++ {
			propertyBinary = "0" + propertyBinary
		}
	}
	length, err := util.BinaryToDecimal(propertyBinary[6:])
	if err != nil {
		return location, errors.New("报文长度解析失败")
	}
	// 解析协议版本号
	version := message[8:10]
	// 解析终端手机号
	terminalPhone := message[10:30]
	// 解析消息流水号
	streamId := message[30:34]
	streamNum, err := util.HexToDecimal(streamId)
	if err != nil {
		return location, errors.New("消息流水号解析失败")
	}
	// 解析消息体
	// 报警标识
	alarmSign := message[34:42]
	// 状态位
	statusSign := message[42:50]
	statusBinary, err := util.HexToBinary(statusSign)
	if err != nil {
		return location, errors.New("状态位解析失败")
	}
	if len(statusBinary) < 32 {
		for i := len(statusBinary); i < 32; i++ {
			statusBinary = "0" + statusBinary
		}
	}

	// 纬度
	latHex := message[50:58]
	latInt, err := util.HexToDecimal(latHex)
	if err != nil {
		return location, errors.New("纬度解析失败")
	}
	lat := float64(latInt) / 1000000
	// 经度
	lonHex := message[58:66]
	lonInt, err := util.HexToDecimal(lonHex)
	if err != nil {
		return location, errors.New("经度解析失败")
	}
	lon := float64(lonInt) / 1000000

	// 高程
	heightHex := message[66:70]
	height, err := util.HexToDecimal(heightHex)
	if err != nil {
		return location, errors.New("高程解析失败")
	}

	// 速度
	speedHex := message[70:74]
	speed, err := util.HexToDecimal(speedHex)
	if err != nil {
		return location, errors.New("速度解析失败")
	}

	// 方向
	directionHex := message[74:78]
	direction, err := util.HexToDecimal(directionHex)
	if err != nil {
		return location, errors.New("方向解析失败")
	}

	// 时间
	tsHex := message[78:90]

	// 附加信息是TLV格式（ID+长度+值），可能包含多个附加项
	// 已解析到90，从90以后到末尾-2（末尾2个字符为校验码）都是附加信息
	// 可见[34:90]是消息体到时间信息的数据，至少56个16进制位即28个字节才能获取基本信息
	var extraInfoList []extraInfoItem
	offset := 90
	end := len(message) - 2 // 去除校验码
	for offset+4 <= end {   // 至少需要4个字符读取ID(2)和Length(2)
		extraInfoId := message[offset : offset+2]
		extraInfoLengthHex := message[offset+2 : offset+4]
		// 附加信息长度为字节数，转十进制
		extraInfoLengthDec, err := util.HexToDecimal(extraInfoLengthHex)
		if err != nil {
			break
		}
		valueLen := int(extraInfoLengthDec) * 2 // 每字节2个hex字符
		if offset+4+valueLen > end {
			break // 长度超出报文边界，截断保护
		}
		extraInfo := message[offset+4 : offset+4+valueLen]
		extraInfoList = append(extraInfoList, extraInfoItem{
			ExtraInfoId:     extraInfoId,
			ExtraInfoLength: extraInfoLengthHex,
			ExtraInfo:       extraInfo,
		})
		offset += 4 + valueLen
	}

	location = x0200Jt808{
		MessageId:     messageId,
		ProperBinary:  propertyBinary,
		Length:        length,
		Version:       version,
		TerminalPhone: terminalPhone,
		StreamNum:     streamNum,
		StreamId:      streamId,
		AlarmSign:     alarmSign,
		StatusSign:    statusSign,
		StatusBinary:  statusBinary,
		Lat:           lat,
		Lon:           lon,
		Horizon:       height,
		Speed:         speed,
		Direction:     direction,
		TsHex:         tsHex,
		ExtraInfo:     extraInfoList,
	}
	return location, nil
}

// 0-成功  1-失败 2消息有误 3-不支持
func CommonReply(paket jt808Head, code string) string {
	messageId := "8001"
	property := paket.ProperBinary[:6] + "0000000101"
	// 小于10位自动补0
	propertyHex := util.BinStrToHexStr(property)
	version := paket.Version
	terminalPhone := paket.TerminalPhone
	streamId := paket.StreamId

	repalyStreamId := paket.StreamId
	repalyId := paket.MessageId
	resultCode := "0" + code

	result := messageId + propertyHex + version + terminalPhone + streamId + repalyStreamId + repalyId + resultCode
	result = strings.ToLower(result)

	checksum := calculateChecksum(result)

	result = result + checksum

	result = strings.ReplaceAll(result, "7e", "7d02")
	result = strings.ReplaceAll(result, "7d", "7d01")
	return "7e" + result + "7e"
}

// 计算校验码
func calculateChecksum(hexStr string) string {
	// 初始化校验码为 0
	var hexList = make([]string, 0)
	// 遍历十六进制字符串，每两个字符一组
	for i := 0; i < len(hexStr); i += 2 {
		// 将两个字符转换为一个字节
		// 将字节转换为二进制数
		binary, err := util.HexToBinary(hexStr[i : i+2])
		if err != nil {
			fmt.Println("Error:", err)
			return ""
		}
		hexList = append(hexList, binary)
	}
	// 异或运算
	checksum := hexList[0]
	for i := 1; i < len(hexList); i++ {
		checksum = binaryXOR(checksum, hexList[i])
	}
	return util.BinStrToHexStr(checksum)
}

func binaryXOR(a, b string) string {
	if len(a) != len(b) {
		panic("Binary strings must have the same length")
	}
	result := ""
	for i := 0; i < len(a); i++ {
		if a[i] == b[i] {
			result += "0"
		} else {
			result += "1"
		}
	}
	return result
}
