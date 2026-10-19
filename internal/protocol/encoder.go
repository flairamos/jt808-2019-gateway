package protocol

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"jt808-gateway/pkg/util"
	"strconv"
	"strings"
	"sync/atomic"
)

// # 平台下行消息编码（0x8100、下发指令）

// t0200HeadLenV2019 2019版消息头长度：17字节 = 34个hex字符
const t0200HeadLenV2019 = 34

type x0200Jt808 struct {
	MessageId     string          // 消息ID
	ProperBinary  string          // 消息体属性对象二进制表示
	Length        int             // 消息属性-消息体长度
	Version       string          // 消息属性-消息版本
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

type Jt808Head struct {
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

// IsSubpackaged 是否分包报文（2013/2011版bit13、2019版bit12）
// ProperBinary为16位二进制串（bit15在前），Version为"1"表示2019版、空串表示2011/2013版
func (h Jt808Head) IsSubpackaged() bool {
	if len(h.ProperBinary) < 16 {
		return false
	}
	if h.Version == "1" {
		return h.ProperBinary[3] == '1'
	}
	return h.ProperBinary[2] == '1'
}

// IsEncrypted 是否加密报文（2013/2011版bit12~10、2019版bit11~10，非0即加密）
func (h Jt808Head) IsEncrypted() bool {
	if len(h.ProperBinary) < 16 {
		return false
	}
	if h.Version == "1" {
		return h.ProperBinary[4] == '1' || h.ProperBinary[5] == '1'
	}
	return h.ProperBinary[3] == '1' || h.ProperBinary[4] == '1' || h.ProperBinary[5] == '1'
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
	// 反转义必须按字节边界处理（见unescapeHexStr）
	return unescapeHexStr(message)
}

// unescapeHexStr 按字节边界反转义hex字符串：仅还原落在偶数位（字节边界）的7d01→7d、7d02→7e。
// 不能对hex文本直接ReplaceAll："7d01"/"7d02"可能横跨字节边界出现（如字节17 d0 21的hex为"17d021"），
// 误还原会导致数据损坏与长度错位。
func unescapeHexStr(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i+2 <= len(s); {
		if i+4 <= len(s) && s[i:i+2] == "7d" && (s[i+2:i+4] == "01" || s[i+2:i+4] == "02") {
			if s[i+2:i+4] == "01" {
				b.WriteString("7d")
			} else {
				b.WriteString("7e")
			}
			i += 4
			continue
		}
		b.WriteString(s[i : i+2])
		i += 2
	}
	// 尾部若残留单个字符（非法输入），直接丢弃
	return b.String()
}

// 根据消息ID判断报文类型
// 只解析消息头分布
// 返回去掉首尾标识符号的报文
func ParsePaketMessageId(raw_message string) (Jt808Head, string, error) {
	var head Jt808Head
	sign := checkSign(raw_message)
	if !sign {
		return head, "", errors.New("报文内容或者格式不对！")
	}
	message := transferMessage(raw_message)
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

	_ = version
	head = Jt808Head{
		MessageId:     messageId,
		ProperBinary:  propertyBinary,
		Length:        length,
		Version:       "1",
		TerminalPhone: terminalPhone,
		StreamNum:     streamNum,
		StreamId:      streamId,
	}
	return head, message, nil
}

// 自动识别协议版本并解析消息头（2013/2019通用入口）
// 依据消息体属性bit14判断：1为2019版（版本标识），0为2017旧版（保留位恒为0）
// 极个别不按标准实现的设备该标志位可能不准，此时请直接调用指定版本的解析函数
// 识别后返回对应版本的head与message：2019版head含版本号，2013版head的Version为空，
// 直接配合现有CommonReply即可生成对应版本的0x8001应答（Version为空时自动拼出2017版消息头）
// 入参为带7e标识位的完整原始帧，内部完成转义还原并校验校验码、消息体长度，
// 损坏/串位的报文直接返回错误，避免脏数据入库
func ParsePaketMessageIdAuto(message string) (Jt808Head, string, error) {
	sign := checkSign(message)
	if !sign {
		return Jt808Head{}, "", errors.New("报文内容或者格式不对！")
	}
	// 先完成转义还原再读取消息体属性，避免属性字节本身被转义（如0x7d/0x7e）导致误判
	inner := transferMessageV2013(message)
	if len(inner) < 8 {
		return Jt808Head{}, "", errors.New("报文长度不足，无法解析消息体属性")
	}
	propertyBinary, err := util.HexToBinary(inner[4:8])
	if err != nil {
		return Jt808Head{}, "", errors.New("报文长度解析失败")
	}
	// 小于16位自动补0
	if len(propertyBinary) < 16 {
		for i := len(propertyBinary); i < 16; i++ {
			propertyBinary = "0" + propertyBinary
		}
	}
	// 校验码与消息体长度校验：拦截损坏/串位的报文
	if err := validateInnerFrame(inner, propertyBinary); err != nil {
		return Jt808Head{}, "", err
	}
	// bit14（16位二进制的第2位，下标1）：1为2019版，0为2017版
	if propertyBinary[1] == '1' {
		return ParsePaketMessageId(message)
	}
	return ParsePaketMessageIdV2013(message)
}

// validateInnerFrame 校验反转义后报文的完整性
// inner：去除首尾7e并完成转义还原后的报文（末尾2个字符为校验码）
// propertyBinary：16位消息体属性二进制串
func validateInnerFrame(inner string, propertyBinary string) error {
	if len(inner) < 4 {
		return errors.New("报文长度不足，无法校验")
	}
	// 校验码：消息头+消息体逐字节异或
	data, err := hex.DecodeString(inner[:len(inner)-2])
	if err != nil {
		return errors.New("报文包含非法十六进制字符")
	}
	var check byte
	for _, b := range data {
		check ^= b
	}
	want, err := hex.DecodeString(inner[len(inner)-2:])
	if err != nil || len(want) != 1 || want[0] != check {
		return fmt.Errorf("报文校验码不符（计算%02x，报文%s）", check, inner[len(inner)-2:])
	}
	// 消息体长度核对：2013版消息头24个hex字符，2019版34个
	headLen := 24
	if propertyBinary[1] == '1' {
		headLen = 34
	}
	bodyLen, err := util.BinaryToDecimal(propertyBinary[6:])
	if err != nil {
		return errors.New("报文长度解析失败")
	}
	if expect := headLen + bodyLen*2 + 2; len(inner) != expect {
		return fmt.Errorf("报文长度与消息体属性不符（应为%d个字符，实际%d个）", expect, len(inner))
	}
	return nil
}

// parseT0200Body 解析0x0200消息体（报警/状态/定位/时间/附加信息TLV）。
// body为消息体hex：从报警标志起、到校验码前；2013与2019消息体格式一致：
// 报警标志(4B)+状态(4B)+纬度(4B)+经度(4B)+高程(2B)+速度(2B)+方向(2B)+时间(6B)+附加信息TLV
func parseT0200Body(body string) (x0200Jt808, error) {
	var p x0200Jt808
	if len(body) < 56 {
		return p, fmt.Errorf("0200消息体长度不足，至少56个hex字符，实际%d个", len(body))
	}
	// 报警标识、状态位
	p.AlarmSign = body[0:8]
	p.StatusSign = body[8:16]
	statusBinary, err := util.HexToBinary(p.StatusSign)
	if err != nil {
		return p, errors.New("状态位解析失败")
	}
	if len(statusBinary) < 32 {
		for i := len(statusBinary); i < 32; i++ {
			statusBinary = "0" + statusBinary
		}
	}
	p.StatusBinary = statusBinary

	// 纬度
	latInt, err := util.HexToDecimal(body[16:24])
	if err != nil {
		return p, errors.New("纬度解析失败")
	}
	p.Lat = float64(latInt) / 1000000
	// 经度
	lonInt, err := util.HexToDecimal(body[24:32])
	if err != nil {
		return p, errors.New("经度解析失败")
	}
	p.Lon = float64(lonInt) / 1000000

	// 高程
	p.Horizon, err = util.HexToDecimal(body[32:36])
	if err != nil {
		return p, errors.New("高程解析失败")
	}
	// 速度
	p.Speed, err = util.HexToDecimal(body[36:40])
	if err != nil {
		return p, errors.New("速度解析失败")
	}
	// 方向
	p.Direction, err = util.HexToDecimal(body[40:44])
	if err != nil {
		return p, errors.New("方向解析失败")
	}
	// 时间
	p.TsHex = body[44:56]

	// 附加信息是TLV格式（ID+长度+值），可能包含多个附加项
	var extraInfoList []extraInfoItem
	offset := 56
	for offset+4 <= len(body) { // 至少需要4个字符读取ID(2)和Length(2)
		extraInfoId := body[offset : offset+2]
		extraInfoLengthHex := body[offset+2 : offset+4]
		// 附加信息长度为字节数，转十进制
		extraInfoLengthDec, err := util.HexToDecimal(extraInfoLengthHex)
		if err != nil {
			break
		}
		valueLen := int(extraInfoLengthDec) * 2 // 每字节2个hex字符
		if offset+4+valueLen > len(body) {
			break // 长度超出报文边界，截断保护
		}
		extraInfo := body[offset+4 : offset+4+valueLen]
		extraInfoList = append(extraInfoList, extraInfoItem{
			ExtraInfoId:     extraInfoId,
			ExtraInfoLength: extraInfoLengthHex,
			ExtraInfo:       extraInfo,
		})
		offset += 4 + valueLen
	}
	p.ExtraInfo = extraInfoList
	return p, nil
}

// mergeT0200Body 把消息体解析结果合并入带消息头的结构
func mergeT0200Body(dst *x0200Jt808, body x0200Jt808) {
	dst.AlarmSign = body.AlarmSign
	dst.StatusSign = body.StatusSign
	dst.StatusBinary = body.StatusBinary
	dst.Lat = body.Lat
	dst.Lon = body.Lon
	dst.Horizon = body.Horizon
	dst.Speed = body.Speed
	dst.Direction = body.Direction
	dst.TsHex = body.TsHex
	dst.ExtraInfo = body.ExtraInfo
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
	// 消息体（从报警标志到校验码前），与2013版格式一致，共用解析
	body, err := parseT0200Body(message[34 : len(message)-2])
	if err != nil {
		return location, err
	}
	location = x0200Jt808{
		MessageId:     messageId,
		ProperBinary:  propertyBinary,
		Length:        length,
		Version:       version,
		TerminalPhone: terminalPhone,
		StreamNum:     streamNum,
		StreamId:      streamId,
	}
	mergeT0200Body(&location, body)
	return location, nil
}

// 0-成功  1-失败 2消息有误 3-不支持 4-报警处理确认
func CommonReply(paket Jt808Head, code string) string {
	messageId := "8001"
	// 消息体属性：沿用原报文高6位（版本标识等），消息体长度固定5（应答流水号2+应答ID2+结果1）
	// 用%04x补齐4位，BinStrToHexStr不带前导0（5会输出"5"而不是"0005"）会导致帧错位
	high6, _ := util.BinaryToDecimal(paket.ProperBinary[:6])
	propertyHex := fmt.Sprintf("%04x", high6<<10+5)
	// 这里的version2019是1补位
	version := "0" + paket.Version
	terminalPhone := paket.TerminalPhone
	// 平台侧流水号独立自增，不复用终端流水号
	platformSerialHex := fmt.Sprintf("%04x", atomic.AddUint32(&platformSerial, 1)&0xffff)

	repalyStreamId := paket.StreamId
	repalyId := paket.MessageId
	resultCode := "0" + code

	result := messageId + propertyHex + version + terminalPhone + platformSerialHex + repalyStreamId + repalyId + resultCode

	result = strings.ToLower(result)

	result = result + calculateChecksum(result)

	return "7e" + escapeHexStr(result) + "7e"
}

func CommonReplyAuto(paket Jt808Head, code string) string {
	if paket.Version == "1" {
		return CommonReply(paket, code)
	} else {
		atoi, _ := strconv.Atoi(code)
		return hex.EncodeToString(CommonReplyV2013(paket, byte(atoi)))
	}
}

// registerAuthCode 注册应答（0x8100）下发的鉴权码，终端鉴权（0x0102）时原样上报校验
const registerAuthCode = "Auth2026x8Kp"

// 终端注册应答 0x8100（字节级构建，按入包消息头版本选择2013/2019头格式）
// code: 0=成功；1=车辆已被注册；2=数据库无该车辆；3=终端已被注册；4=数据库无该终端
// 消息体：应答流水号(2) + 结果(1) + 鉴权码(变长)
func RegisterReply(paket Jt808Head, code string) string {
	atoi, _ := strconv.Atoi(code)
	result := byte(atoi)

	ackSerial, _ := hex.DecodeString(paket.StreamId)  // 应答流水号 = 终端注册包流水号
	phone, _ := hex.DecodeString(paket.TerminalPhone) // 终端手机号BCD
	auth := []byte(registerAuthCode)

	// 消息体：应答流水号(2) + 结果(1) + 鉴权码
	body := make([]byte, 0, 2+1+len(auth))
	body = append(body, ackSerial...)
	body = append(body, result)
	body = append(body, auth...)

	// 消息头
	var msgID, prop, serial [2]byte
	binary.BigEndian.PutUint16(msgID[:], 0x8100)
	binary.BigEndian.PutUint16(serial[:], uint16(atomic.AddUint32(&platformSerial, 1)))

	head := make([]byte, 0, 17)
	if paket.Version == "1" {
		// 2019版：ID(2) + 属性(2，bit14=1标识2019) + 版本号(1) + 手机号BCD(10) + 平台流水号(2)
		binary.BigEndian.PutUint16(prop[:], 0x4000|uint16(len(body)))
		head = append(head, msgID[:]...)
		head = append(head, prop[:]...)
		head = append(head, 0x01)
		head = append(head, phone...)
		head = append(head, serial[:]...)
	} else {
		// 2013/2011版：ID(2) + 属性(2) + 手机号BCD(6) + 平台流水号(2)
		binary.BigEndian.PutUint16(prop[:], uint16(len(body)))
		head = append(head, msgID[:]...)
		head = append(head, prop[:]...)
		head = append(head, phone...)
		head = append(head, serial[:]...)
	}
	return hex.EncodeToString(buildJt808Frame(head, body))
}

// buildJt808Frame 字节级拼帧：消息头+消息体 → 追加异或校验码 → 转义 → 包首尾0x7E。
// 校验码用%02x思路直接写字节，天然无丢前导零问题。
func buildJt808Frame(head, body []byte) []byte {
	pkt := make([]byte, 0, len(head)+len(body)+1)
	pkt = append(pkt, head...)
	pkt = append(pkt, body...)
	var check byte
	for _, b := range pkt {
		check ^= b
	}
	pkt = append(pkt, check)

	// 字节级转义：先判0x7D再判0x7E，天然不会双重转义
	out := make([]byte, 0, len(pkt)+4)
	for _, b := range pkt {
		switch b {
		case 0x7D:
			out = append(out, 0x7D, 0x01)
		case 0x7E:
			out = append(out, 0x7D, 0x02)
		default:
			out = append(out, b)
		}
	}
	return append(append([]byte{0x7E}, out...), 0x7E)
}

// 按字符特殊字符转义
// 按字节边界（2个hex字符）单遍转义
func escapeHexStr(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i+2 <= len(s); i += 2 {
		switch s[i : i+2] {
		case "7d":
			b.WriteString("7d01")
		case "7e":
			b.WriteString("7d02")
		default:
			b.WriteString(s[i : i+2])
		}
	}
	return b.String()
}

// 计算校验码：对hex字符串逐字节异或
// 恒定输出2个hex字符（%02x）：此前BinStrToHexStr不带前导0，校验码<0x10时会输出"7"而非"07"，
// 导致拼出的应答帧缺字节（约1/16概率），设备按标准校验会丢弃
func calculateChecksum(hexStr string) string {
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		fmt.Println("Error:", err)
		return ""
	}
	var check byte
	for _, b := range data {
		check ^= b
	}
	return fmt.Sprintf("%02x", check)
}
