package protocol

// 0x0704批量定位上报回归测试（报文取自2026-10-19生产日志，39.144.70.x聚合设备）

import (
	"encoding/hex"
	"strings"
	"testing"
)

// 单条数据项的真实帧：2019版头，1项，补报类型
const frame0704Single = "7e070440370100000000016083085176058e000101003200000000000c000301322210069412c000030000000e26101909081001040000784330011a31011de10200fdfa0400000000a47e"

// 15条数据项的真实帧：2019版头，全部为同一终端的3分钟间隔补报轨迹
const frame0704Multi = "7e0704430f01000000000133073370740017000f01003200000000004800030130e61f0690a05f0020010e001d26101907415401040003fddd03020118250400000000300163310112003200000000004800030130f0430690a2a9001901a4000126101907422501040003fde0030201a4250400000000300109310112003200000000000800030130f83b0690a2d3000f0000016326101907425501040003fde203020000250400000000300109310112003200000000000800030130f8310690a1fc000f000000be26101907432501040003fde203020000250400000000300163310112003200000000000800030130f8310690a1fc000f000000be26101907435501040003fde203020000250400000000300163310112003200000000000800030130f8240690a210000d0000004426101907442501040003fde203020000250400000000300163310112003200000000000800020130f8240690a210000d0000004426101907444501040003fde203020000250400000000300163310110003200000000000800020130f8240690a210000d0000004426101907474401040003fde203020000250400000000300163310112003200000000000800020130f8240690a210000d0000004426101907504501040003fde203020000250400000000300163310112003200000000000800020130f8240690a210000d0000004426101907534501040003fde203020000250400000000300163310112003200000000000800020130f8240690a210000d0000004426101907564501040003fde203020000250400000000300163310111003200000000000800020130f8240690a210000d0000004426101907594501040003fde203020000250400000000300163310112003200000000000800020130f8240690a210000d0000004426101908024501040003fde203020000250400000000300163310112003200000000000800020130f8240690a210000d0000004426101908054701040003fde203020000250400000000300163310111003200000000000800020130f8240690a210000d0000004426101908084701040003fde203020000250400000000300163310111fa7e"

func parse0704(t *testing.T, frame string) (Jt808Head, []Location) {
	t.Helper()
	head, msg, err := ParsePaketMessageIdAuto(frame)
	if err != nil {
		t.Fatalf("解析消息头失败: %v", err)
	}
	if head.MessageId != "0704" {
		t.Fatalf("消息ID=%s，期望0704", head.MessageId)
	}
	locs, err := GetLocationBatchAuto(head, msg)
	if err != nil {
		t.Fatalf("解析批量定位失败: %v", err)
	}
	return head, locs
}

// 真实单条帧：精确核对各字段
func Test0704SingleItemFromLog(t *testing.T) {
	_, locs := parse0704(t, frame0704Single)
	if len(locs) != 1 {
		t.Fatalf("解析出%d条，期望1条", len(locs))
	}
	l := locs[0]
	if l.ImeiNum != "00000000016083085176" {
		t.Fatalf("imei=%s", l.ImeiNum)
	}
	if l.Lat != 20.062736 || l.Lon != 110.367424 {
		t.Fatalf("坐标 lat=%f lon=%f", l.Lat, l.Lon)
	}
	if l.Altitude != 3 || l.Speed != "0" || l.Direction != "14" {
		t.Fatalf("alt=%d speed=%s dir=%s", l.Altitude, l.Speed, l.Direction)
	}
	if l.CreateTime != "2026-10-19 09:08:10" {
		t.Fatalf("time=%s", l.CreateTime)
	}
}

// 真实15条帧：逐项校验归属终端与时间单调性
func Test0704MultiItemFromLog(t *testing.T) {
	head, locs := parse0704(t, frame0704Multi)
	if len(locs) != 15 {
		t.Fatalf("解析出%d条，期望15条", len(locs))
	}
	for i, l := range locs {
		if l.ImeiNum != head.TerminalPhone {
			t.Fatalf("第%d条imei=%s，期望归属上报终端%s", i, l.ImeiNum, head.TerminalPhone)
		}
		if l.Lat < 18 || l.Lat > 21 || l.Lon < 108 || l.Lon > 112 {
			t.Fatalf("第%d条坐标异常 lat=%f lon=%f", i, l.Lat, l.Lon)
		}
		if i > 0 && l.CreateTime < locs[i-1].CreateTime {
			t.Fatalf("第%d条时间回退: %s < %s", i, l.CreateTime, locs[i-1].CreateTime)
		}
	}
	first, last := locs[0], locs[14]
	if first.Lat != 19.981855 || first.Lon != 110.141535 || first.CreateTime != "2026-10-19 07:41:54" {
		t.Fatalf("首条 lat=%f lon=%f time=%s", first.Lat, first.Lon, first.CreateTime)
	}
	if last.CreateTime != "2026-10-19 08:08:47" {
		t.Fatalf("末条 time=%s", last.CreateTime)
	}
}

// 2013版头0704（用现场2013版0200报文体合成）：手机号应补零到20位
func Test0704V2013Synthetic(t *testing.T) {
	// 取回归测试中的现场2013版0200帧，剥出消息体
	frames := splitFramesHex(t, rawBlob)
	f1 := frames[0]
	inner1 := f1[2 : len(f1)-4] // 去首尾7e和校验码（未反转义，帧1无转义字符）
	body := inner1[24:]         // 2013消息头24字符之后即0200消息体

	item := "0070" + body // 数据项长度前缀(112B)
	b704 := "0002" + "00" + item + item
	head704 := "0704" + "00e7" + "013924072039" + "e36b" // 属性：体长231B(0xE7)，2013版
	full := head704 + b704
	frame := "7e" + escapeHexStr(full+calculateChecksum(full)) + "7e"

	h, locs := parse0704(t, frame)
	if h.Version != "" {
		t.Fatalf("版本=%q，期望2013(空)", h.Version)
	}
	if len(locs) != 2 {
		t.Fatalf("解析出%d条，期望2条", len(locs))
	}
	for i, l := range locs {
		if l.ImeiNum != "00000000"+h.TerminalPhone {
			t.Fatalf("第%d条imei=%s，期望补零后的%s", i, l.ImeiNum, h.TerminalPhone)
		}
		if l.CreateTime != "2026-10-15 16:01:05" {
			t.Fatalf("第%d条time=%s", i, l.CreateTime)
		}
	}
}

// 现场聚合器2013版0704真实帧（3项补报）：手机号12位补零、逐项时间正确
const frame0704V2013Real = "7e0704009f0412688358139fb7000301003200000000000c00c201310ee20692a138002900000160261019090121010400084b200302000025040000000030011931011e003200000000000c00c201310ee20692a138002900000160261019090421010400084b200302000025040000000030011931011c003200000000000c00c201310ee20692a138002900000160261019090721010400084b200302000025040000000030011a31011c237e"

func Test0704V2013RealAggregatorFrame(t *testing.T) {
	h, locs := parse0704(t, frame0704V2013Real)
	if h.Version != "" {
		t.Fatalf("版本=%q，期望2013(空)", h.Version)
	}
	if len(locs) != 3 {
		t.Fatalf("解析出%d条，期望3条", len(locs))
	}
	wantTime := []string{"2026-10-19 09:01:21", "2026-10-19 09:04:21", "2026-10-19 09:07:21"}
	for i, l := range locs {
		if l.ImeiNum != "00000000041268835813" {
			t.Fatalf("第%d条imei=%s", i, l.ImeiNum)
		}
		if l.CreateTime != wantTime[i] {
			t.Fatalf("第%d条time=%s，期望%s", i, l.CreateTime, wantTime[i])
		}
		if l.Lat < 19 || l.Lat > 20 || l.Lon < 110 || l.Lon > 111 {
			t.Fatalf("第%d条坐标异常 lat=%f lon=%f", i, l.Lat, l.Lon)
		}
	}
}

// 数据项长度越界（截断/损坏）：安全返回已解析项，不panic
func Test0704TruncationGuard(t *testing.T) {
	// 单条帧中数据项长度0032改为00ff（超出剩余报文）
	tampered := strings.Replace(frame0704Single, "0001010032", "00010100ff", 1)
	h, msg, err := ParsePaketMessageIdAuto(tampered)
	if err != nil {
		t.Skipf("篡改后帧校验不过（预期内）: %v", err)
	}
	locs, err := GetLocationBatchAuto(h, msg)
	if err == nil && len(locs) > 0 {
		// 校验码仍通过但数据项越界的极端情况：允许0项+错误，或部分项
		t.Logf("越界项返回%d条 err=%v", len(locs), err)
	}
}

// 消息体纯垃圾：返回错误而不是崩溃
func Test0704GarbageBody(t *testing.T) {
	// 拼一个头合法、体为垃圾的帧（属性长度必须与实际消息体一致：2+1+4×4=19字节=0x13）
	head704 := "0704" + "0013" + "013924072039" + "e36b"
	body := "0003" + "00" + "0004dead" + "0004beef" + "0004cafe" + "0004baad"
	full := head704 + body
	frame := "7e" + escapeHexStr(full+calculateChecksum(full)) + "7e"
	h, msg, err := ParsePaketMessageIdAuto(frame)
	if err != nil {
		t.Fatalf("消息头解析失败: %v", err)
	}
	if _, err := GetLocationBatchAuto(h, msg); err == nil {
		t.Fatal("垃圾数据项不应解析成功")
	}
}

// 0704应答帧可被设备标准校验（8001应答含ackId=0704、结果0）
func Test0704ReplyStructure(t *testing.T) {
	head, _ := parse0704(t, frame0704Single)
	resp := CommonReplyAuto(head, "0")
	raw, err := hex.DecodeString(resp)
	if err != nil {
		t.Fatalf("应答hex解码失败: %v resp=%s", err, resp)
	}
	inner := unescapeBytes(raw[1 : len(raw)-1])
	// 8001+属性(2)+版本(1)+手机号(10)+平台流水号(2)+应答流水号(2)+应答ID(2)+结果(1)+校验(1)=23
	if len(inner) != 23 {
		t.Fatalf("应答长度=%d，期望23: %s", len(inner), resp)
	}
	// 应答ID位于应答流水号之后的[19:21]
	if got := hex.EncodeToString(inner[19:21]); got != "0704" {
		t.Fatalf("应答ID=%s，期望0704", got)
	}
	if inner[21] != 0 {
		t.Fatalf("结果码=%d，期望0", inner[21])
	}
	var check byte
	for _, b := range inner[:len(inner)-1] {
		check ^= b
	}
	if check != inner[len(inner)-1] {
		t.Fatalf("校验码不符: %s", resp)
	}
}
