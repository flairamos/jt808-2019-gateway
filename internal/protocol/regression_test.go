package protocol

// 回归测试：现场报文解析、完整性校验、转义往返、下行应答结构

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

// 现场抓包：聚合设备一条TCP连接一次发出的4帧粘连报文（4台车的2013版0x0200定位上报，均合法）
const rawBlob = "7e02000070013924072039e36b00000000000800030124e5e6067f9652006f0104011b2610151601050104000b8d5d0202000003020118140480000000150400000000160400000000170200002504000000002b040000000030011d31010eeb0d000300d400000600f880000000ef0d00000049240049249200001103587e7e02000044013901621187e36c000000000008000201191d4206812050002f00000098261015160106010400020e170302000025040000000030011f31010ef302000bfc0c000004a60001000000000000bc7e7e02000044013901620325e36d00000000000800030125344b0690401000c70000009826101516010601040000fd40020200bc03020000e1020a042504000000002a0200002b040000000030011931010f977e7e0200003a013921033017e36e00000000000c00030124ffa40694e3270016000000c12610151601050104003c674c02020cb2030200002504000000002a020800300114310117e77e"

// splitFramesHex 按字节0x7E切帧（转义后帧内无裸0x7E，字节级切分安全），返回每帧hex串（含首尾7e）
func splitFramesHex(t *testing.T, blob string) []string {
	t.Helper()
	raw, err := hex.DecodeString(blob)
	if err != nil {
		t.Fatalf("hex解码失败: %v", err)
	}
	var frames []string
	pos := -1
	for i, b := range raw {
		if b != 0x7e {
			continue
		}
		if pos >= 0 && i > pos+1 {
			frames = append(frames, hex.EncodeToString(raw[pos:i+1]))
		}
		pos = i
	}
	return frames
}

// unescapeBytes 测试用：字节级反转义
func unescapeBytes(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		if b[i] == 0x7d && i+1 < len(b) && (b[i+1] == 0x01 || b[i+1] == 0x02) {
			if b[i+1] == 0x01 {
				out = append(out, 0x7d)
			} else {
				out = append(out, 0x7e)
			}
			i += 2
			continue
		}
		out = append(out, b[i])
		i++
	}
	return out
}

// 现场的4帧各自独立解析都必须成功，且不应误判分包/加密
func TestUserFramesAllParse(t *testing.T) {
	frames := splitFramesHex(t, rawBlob)
	if len(frames) != 4 {
		t.Fatalf("期望切出4帧，实际%d帧", len(frames))
	}
	phones := []string{"013924072039", "013901621187", "013901620325", "013921033017"}
	for i, f := range frames {
		head, msg, err := ParsePaketMessageIdAuto(f)
		if err != nil {
			t.Fatalf("帧%d解析失败: %v", i+1, err)
		}
		if head.TerminalPhone != phones[i] {
			t.Fatalf("帧%d终端号=%s，期望%s", i+1, head.TerminalPhone, phones[i])
		}
		if head.IsSubpackaged() || head.IsEncrypted() {
			t.Fatalf("帧%d被误判为分包/加密", i+1)
		}
		loc, err := GetLocationAuto(head, msg)
		if err != nil {
			t.Fatalf("帧%d定位解析失败: %v", i+1, err)
		}
		if loc.ImeiNum != "00000000"+phones[i] {
			t.Fatalf("帧%d imei=%s", i+1, loc.ImeiNum)
		}
	}
	// 帧2带fc附加信息：举升=-1（附加信息TLV解析回归）
	head2, msg2, err := ParsePaketMessageIdAuto(frames[1])
	if err != nil {
		t.Fatalf("帧2解析失败: %v", err)
	}
	loc2, err := GetLocationAuto(head2, msg2)
	if err != nil {
		t.Fatal(err)
	}
	if loc2.Lift != -1 {
		t.Fatalf("帧2举升=%d，期望-1", loc2.Lift)
	}
}

func mustHead(t *testing.T, frame string) Jt808Head {
	t.Helper()
	head, _, err := ParsePaketMessageIdAuto(frame)
	if err != nil {
		t.Fatalf("解析消息头失败: %v", err)
	}
	return head
}

// 校验码被篡改必须被拦截
func TestAutoRejectsBadChecksum(t *testing.T) {
	frames := splitFramesHex(t, rawBlob)
	f := frames[0]
	// 帧尾：...校验码(1字节)+7e，把校验码改成00
	tampered := f[:len(f)-4] + "00" + f[len(f)-2:]
	if _, _, err := ParsePaketMessageIdAuto(tampered); err == nil {
		t.Fatal("校验码被篡改后仍解析成功")
	}
}

// 消息体长度字段与实际不符必须被拦截
func TestAutoRejectsBadLength(t *testing.T) {
	frames := splitFramesHex(t, rawBlob)
	f := frames[0] // 属性0070：声明体长112字节
	tampered := strings.Replace(f, "02000070013924", "02000071013924", 1)
	if _, _, err := ParsePaketMessageIdAuto(tampered); err == nil {
		t.Fatal("消息体长度字段被篡改后仍解析成功")
	}
	// 多帧粘连的整段也不能再被当成一帧解析（修复前会"成功"解析第一帧并丢弃其余帧）
	if _, _, err := ParsePaketMessageIdAuto(rawBlob); err == nil {
		t.Fatal("4帧粘连的整段报文不应被当作单帧解析成功")
	}
}

// 反转义必须按字节边界处理：跨字节边界的伪7d01/7d02不能被还原
func TestUnescapeHexStrByteBoundary(t *testing.T) {
	// 字节17 d0 21的hex为"17d021"，其中下标1处出现伪"7d02"
	if got := unescapeHexStr("17d021"); got != "17d021" {
		t.Fatalf("跨字节边界的伪7d02被误还原: %s", got)
	}
	// 字节a7 d0 1b的hex为"a7d01b"，其中下标1处出现伪"7d01"
	if got := unescapeHexStr("a7d01b"); got != "a7d01b" {
		t.Fatalf("跨字节边界的伪7d01被误还原: %s", got)
	}
	if got := unescapeHexStr("7d027d01"); got != "7e7d" {
		t.Fatalf("正常反转义错误: %s", got)
	}
	if got := unescapeHexStr("7d017d02"); got != "7d7e" {
		t.Fatalf("正常反转义错误: %s", got)
	}
}

// 消息体中含0x7d/0x7e字节及伪转义序列时，转义→解析全链路往返必须无损
func TestEscapeRoundTripWithSpecialBytes(t *testing.T) {
	body := "00000000" + // 报警
		"00000003" + // 状态
		"0124e5e6" + // 纬度
		"067f9652" + // 经度
		"006f" + // 高程
		"0104" + // 速度
		"011b" + // 方向
		"261015160105" + // 时间
		"fc027d7d" + // 举升，值含0x7d字节
		"fd027e01" + // 闭合，值含0x7e字节
		"fe0317d021" // 干扰项：hex文本含跨字节边界的伪"7d02"
	head := "0200" + fmt.Sprintf("%04x", len(body)/2) + "013924072039" + "e36b"
	full := head + body
	frame := "7e" + escapeHexStr(full+calculateChecksum(full)) + "7e"

	h, msg, err := ParsePaketMessageIdAuto(frame)
	if err != nil {
		t.Fatalf("含转义字节的报文解析失败: %v", err)
	}
	loc, err := GetLocationAuto(h, msg)
	if err != nil {
		t.Fatalf("定位解析失败: %v", err)
	}
	if loc.Lift != 0x7d7d {
		t.Fatalf("举升=%d，期望%d（0x7d7d未正确往返）", loc.Lift, 0x7d7d)
	}
	if loc.Close != 0x7e01 {
		t.Fatalf("闭合=%d，期望%d（0x7e01未正确往返）", loc.Close, 0x7e01)
	}
}

// 2019版平台通用应答：反复构建覆盖校验码<0x10的场景（修复前约1/16的应答缺校验码字节）
func TestCommonReplyWellFormed(t *testing.T) {
	head := Jt808Head{
		MessageId:     "0200",
		ProperBinary:  "0100000000000101", // bit14=1：2019版
		Version:       "1",
		TerminalPhone: "00000000013924072039",
		StreamId:      "e36b",
	}
	for i := 0; i < 64; i++ {
		resp := CommonReply(head, "0")
		raw, err := hex.DecodeString(resp)
		if err != nil {
			t.Fatalf("第%d次应答hex解码失败（帧残缺）: %v resp=%s", i+1, err, resp)
		}
		if raw[0] != 0x7e || raw[len(raw)-1] != 0x7e {
			t.Fatalf("第%d次应答首尾标识位错误: %s", i+1, resp)
		}
		inner := unescapeBytes(raw[1 : len(raw)-1])
		// 8001(2)+属性(2)+版本(1)+手机号(10)+平台流水号(2)+应答流水号(2)+应答ID(2)+结果(1)+校验码(1)=23字节
		if len(inner) != 23 {
			t.Fatalf("第%d次应答反转义后长度=%d，期望23: %s", i+1, len(inner), resp)
		}
		if inner[0] != 0x80 || inner[1] != 0x01 {
			t.Fatalf("第%d次应答消息ID错误: %s", i+1, resp)
		}
		var check byte
		for _, b := range inner[:len(inner)-1] {
			check ^= b
		}
		if check != inner[len(inner)-1] {
			t.Fatalf("第%d次应答校验码不符: %s", i+1, resp)
		}
	}
}

// 注册应答0x8100结构：此前缺消息体属性/手机号/平台流水号，设备无法解析
func TestRegisterReplyStructure(t *testing.T) {
	// 2013版
	head := Jt808Head{
		MessageId:     "0100",
		ProperBinary:  "0010000000011100",
		Version:       "",
		TerminalPhone: "013924072039",
		StreamId:      "e36b",
	}
	resp := RegisterReply(head, "0")
	raw, err := hex.DecodeString(resp)
	if err != nil {
		t.Fatalf("hex解码失败: %v resp=%s", err, resp)
	}
	if raw[0] != 0x7e || raw[len(raw)-1] != 0x7e {
		t.Fatalf("首尾标识位错误: %s", resp)
	}
	inner := unescapeBytes(raw[1 : len(raw)-1])
	// 头12（8100+属性+手机号6+平台流水号2）+ 体15（应答流水号2+结果1+鉴权码12）+ 校验1 = 28
	if len(inner) != 28 {
		t.Fatalf("2013注册应答长度=%d，期望28: %s", len(inner), resp)
	}
	if inner[0] != 0x81 || inner[1] != 0x00 {
		t.Fatalf("消息ID不是8100: %s", resp)
	}
	if prop := uint16(inner[2])<<8 | uint16(inner[3]); prop != 15 {
		t.Fatalf("属性=%d，期望15（消息体长度）", prop)
	}
	if got := hex.EncodeToString(inner[4:10]); got != "013924072039" {
		t.Fatalf("手机号错误: %s", got)
	}
	if got := hex.EncodeToString(inner[12:14]); got != "e36b" {
		t.Fatalf("应答流水号错误: %s", got)
	}
	if inner[14] != 0 {
		t.Fatalf("结果码不是0: %d", inner[14])
	}
	if got := string(inner[15:27]); got != "Auth2026x8Kp" {
		t.Fatalf("鉴权码错误: %q", got)
	}
	var check byte
	for _, b := range inner[:len(inner)-1] {
		check ^= b
	}
	if check != inner[len(inner)-1] {
		t.Fatalf("校验码不符: %s", resp)
	}

	// 2019版
	head2019 := head
	head2019.Version = "1"
	head2019.TerminalPhone = "00000000013924072039"
	resp = RegisterReply(head2019, "0")
	raw, _ = hex.DecodeString(resp)
	inner = unescapeBytes(raw[1 : len(raw)-1])
	// 头17（8100+属性+版本1+手机号10+平台流水号2）+ 体15 + 校验1 = 33
	if len(inner) != 33 {
		t.Fatalf("2019注册应答长度=%d，期望33: %s", len(inner), resp)
	}
	if prop := uint16(inner[2])<<8 | uint16(inner[3]); prop != 0x4000|15 {
		t.Fatalf("2019属性=%04x，期望400f（bit14=1+长度15）", prop)
	}
	if inner[4] != 0x01 {
		t.Fatalf("版本号字节不是01: %02x", inner[4])
	}
	if got := hex.EncodeToString(inner[5:15]); got != "00000000013924072039" {
		t.Fatalf("手机号错误: %s", got)
	}
}

// 分包/加密标志识别（2013版bit13/12-10，2019版bit12/11-10）
func TestSubpackageAndEncryptDetect(t *testing.T) {
	// 2013分包bit13=1（属性0x201C）
	if h := (Jt808Head{ProperBinary: "0010000000011100"}); !h.IsSubpackaged() || h.IsEncrypted() {
		t.Fatal("2013分包识别错误")
	}
	// 2013加密bit12=1（属性0x101C）
	if h := (Jt808Head{ProperBinary: "0001000000011100"}); !h.IsEncrypted() || h.IsSubpackaged() {
		t.Fatal("2013加密识别错误")
	}
	// 2019分包bit12=1（属性0x501C，bit14=1）
	if h := (Jt808Head{ProperBinary: "0101000000011100", Version: "1"}); !h.IsSubpackaged() || h.IsEncrypted() {
		t.Fatal("2019分包识别错误")
	}
	// 2019加密bit11=1（属性0x481C）
	if h := (Jt808Head{ProperBinary: "0100100000011100", Version: "1"}); !h.IsEncrypted() || h.IsSubpackaged() {
		t.Fatal("2019加密识别错误")
	}
	// 普通报文两者皆否
	frames := splitFramesHex(t, rawBlob)
	if h := mustHead(t, frames[0]); h.IsSubpackaged() || h.IsEncrypted() {
		t.Fatal("普通报文被误判")
	}
}
