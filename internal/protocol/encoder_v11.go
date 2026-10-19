package protocol

import (
	"errors"
	"fmt"
	"jt808-gateway/pkg/util"
	"strings"
)

// # JT808-2011（旧版协议）解析与应答编码
//
// JT/T 808-2011（含2013修改单）为旧版协议，与2019版的差异只在消息头，消息体（如0x0200定位）格式完全一致：
//
//	2019版消息头(17字节/34个hex字符)：消息ID(2) + 消息体属性(2) + 协议版本号(1) + 终端手机号BCD[10] + 消息流水号(2)
//	2011版消息头(12字节/24个hex字符)：消息ID(2) + 消息体属性(2) +               终端手机号BCD[6]  + 消息流水号(2)
//
// 即：2011版没有协议版本号字节，终端手机号为6字节BCD（12个hex字符）。
// 消息体属性两个版本定义一致：bit0~9消息体长度、bit10~12数据加密方式、bit13是否分包；
// bit14在2019版为版本标识（1表示2019版），在2011版为保留位（恒为0），可据此自动识别版本，见ParsePaketMessageIdV2011Auto。
// 注：bit13=1时消息头后面的消息包封装项（消息总包数+包序号）本实现未处理，与2019版现有实现保持一致。
//
// 说明：报文格式与 encoder_version.go 中按项目习惯命名的"2017版"完全一致
// （"2017"并非官方标准版本号，旧版格式即出自JT/T 808-2011/2013），两个文件可任选其一使用。
//
// 复用encoder.go中的 jt808Head / x0200Jt808 / extraInfoItem 结构体及 checkSign / calculateChecksum 函数，
// 2011版解析结果中 Version 为空字符串。

const (
	// jt808HeadLenV2011 2011版消息头长度：12字节 = 24个hex字符
	jt808HeadLenV2011 = 24
	// t0200MinLenV2011 2011版0x0200最小报文长度：消息头(24) + 报警(8) + 状态(8) + 纬度(8) + 经度(8) + 高程(4) + 速度(4) + 方向(4) + 时间(12) = 80
	t0200MinLenV2011 = 80
)

// 转义还原（反转义）：7d02→7e、7d01→7d，按字节边界单遍处理（见encoder.go的unescapeHexStr）
// 不能对hex文本直接ReplaceAll："7d01"/"7d02"可能横跨字节边界出现（如字节17 d0 21的hex为"17d021"），
// 误还原会导致数据损坏与长度错位
func transferMessageV2011(message string) string {
	// 去除首尾空白字符（如\r\n）和所有空格，统一转小写
	message = strings.ToLower(strings.TrimSpace(message))
	message = strings.ReplaceAll(message, " ", "")
	// 部分设备会发送0x/0X前缀，去除
	if len(message) >= 2 && message[:2] == "0x" {
		message = message[2:]
	}
	// 剥离首尾7e标识符（防止"7e"这类极短输入切片越界）
	if len(message) < 4 {
		return ""
	}
	message = message[2 : len(message)-2]
	return unescapeHexStr(message)
}

// 转义（下行报文用）：复用encoder.go中按字节边界单遍转义的escapeHexStr
func escapeMessageV2011(message string) string {
	return escapeHexStr(message)
}

// 解析2011版消息头
// 入参为带7e标识位的原始报文（兼容0x前缀、空格、大小写混用），返回：
//   - head：消息头（Version为空，2011版没有协议版本号字段）
//   - message：去除7e并完成转义还原后的报文（末尾仍含2位校验码），
//     可直接传入 t0200JT808PaketV2011 / GetLocationV2011 解析0x0200消息体
func ParsePaketMessageIdV2011(message string) (Jt808Head, string, error) {
	var head Jt808Head
	sign := checkSign(message)
	if !sign {
		return head, "", errors.New("报文内容或者格式不对！")
	}
	message = transferMessageV2011(message)
	// 校验报文最小长度：仅解析消息头至少需要24个hex字符
	if len(message) < jt808HeadLenV2011 {
		return head, "", fmt.Errorf("报文长度不足，消息头至少需要%d个字符，实际%d个", jt808HeadLenV2011, len(message))
	}
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
	// 解析终端手机号（6字节BCD，12个hex字符；2019版为10字节且其前多1字节版本号）
	terminalPhone := message[8:20]
	// 解析消息流水号
	streamId := message[20:24]
	streamNum, err := util.HexToDecimal(streamId)
	if err != nil {
		return head, "", errors.New("消息流水号解析失败")
	}

	head = Jt808Head{
		MessageId:     messageId,
		ProperBinary:  propertyBinary,
		Length:        length,
		Version:       "", // 2011版消息头没有协议版本号字段
		TerminalPhone: terminalPhone,
		StreamNum:     streamNum,
		StreamId:      streamId,
	}
	return head, message, nil
}

// 这里只做2011版定位信息报文解析
// 入参为去除7e标识符并完成转义还原后的报文（末尾仍含2位校验码），即ParsePaketMessageIdV2011返回的message
func t0200JT808PaketV2011(message string) (x0200Jt808, error) {
	var location x0200Jt808
	// 校验报文最小长度：消息头(24) + 报警(8) + 状态(8) + 纬度(8) + 经度(8) + 高程(4) + 速度(4) + 方向(4) + 时间(12) = 80
	if len(message) < t0200MinLenV2011 {
		return location, fmt.Errorf("报文长度不足，至少需要%d个字符，实际%d个", t0200MinLenV2011, len(message))
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
	// 解析终端手机号（6字节BCD）
	terminalPhone := message[8:20]
	// 解析消息流水号
	streamId := message[20:24]
	streamNum, err := util.HexToDecimal(streamId)
	if err != nil {
		return location, errors.New("消息流水号解析失败")
	}
	// 消息体（从报警标志到校验码前），与2019版格式一致，共用解析
	body, err := parseT0200Body(message[24 : len(message)-2])
	if err != nil {
		return location, err
	}
	location = x0200Jt808{
		MessageId:     messageId,
		ProperBinary:  propertyBinary,
		Length:        length,
		Version:       "", // 2011版消息头无协议版本号字段
		TerminalPhone: terminalPhone,
		StreamNum:     streamNum,
		StreamId:      streamId,
	}
	mergeT0200Body(&location, body)
	return location, nil
}

// 2011版平台通用应答（0x8001）
// 消息体：应答流水号(2) + 应答ID(2) + 结果(1)，固定5字节
// 与CommonReply(2019版)的差异：消息头中没有协议版本号字节
// code: 0-成功  1-失败 2消息有误 3-不支持 4-报警处理确认
func CommonReplyV2011(paket Jt808Head, code string) string {
	messageId := "8001"
	// 消息体属性：保留原报文的bit10~15（加密方式/分包标识等），消息体长度置为5
	// 注意这里不能用BinStrToHexStr（输出不带前导0，如5会输出"5"而不是"0005"），用%04x补齐4位
	high6, _ := util.BinaryToDecimal(paket.ProperBinary[:6])
	propertyHex := fmt.Sprintf("%04x", high6<<10+5)
	terminalPhone := paket.TerminalPhone
	streamId := paket.StreamId

	repalyStreamId := paket.StreamId
	repalyId := paket.MessageId
	resultCode := "0" + code

	result := messageId + propertyHex + terminalPhone + streamId + repalyStreamId + repalyId + resultCode
	result = strings.ToLower(result)

	checksum := calculateChecksum(result)
	result = result + checksum

	result = escapeMessageV2011(result)
	return "7e" + result + "7e"
}
