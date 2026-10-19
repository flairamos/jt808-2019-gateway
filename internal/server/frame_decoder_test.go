package server

// 回归测试：TCP分帧、半帧缓冲、端到端粘包/拆包/连发（报文取自现场抓包）

import (
	"encoding/hex"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"jt808-gateway/internal/protocol"
	"jt808-gateway/internal/sink"
)

// 现场抓包：聚合设备一条TCP连接一次发出的4帧粘连报文（4台车的2013版0x0200定位上报，均合法）
const rawPacket = "7e02000070013924072039e36b00000000000800030124e5e6067f9652006f0104011b2610151601050104000b8d5d0202000003020118140480000000150400000000160400000000170200002504000000002b040000000030011d31010eeb0d000300d400000600f880000000ef0d00000049240049249200001103587e7e02000044013901621187e36c000000000008000201191d4206812050002f00000098261015160106010400020e170302000025040000000030011f31010ef302000bfc0c000004a60001000000000000bc7e7e02000044013901620325e36d00000000000800030125344b0690401000c70000009826101516010601040000fd40020200bc03020000e1020a042504000000002a0200002b040000000030011931010f977e7e0200003a013921033017e36e00000000000c00030124ffa40694e3270016000000c12610151601050104003c674c02020cb2030200002504000000002a020800300114310117e77e"

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("hex解码失败: %v", err)
	}
	return b
}

// 粘包的4帧应全部切出
func TestExtractFramesConcatenated(t *testing.T) {
	data := mustHex(t, rawPacket)
	frames, rest := extractFrames(data)
	if len(frames) != 4 {
		t.Fatalf("期望4帧，实际%d帧", len(frames))
	}
	// 末尾的7e可能是下一帧的帧头，应保留在缓冲中
	if len(rest) != 1 || rest[0] != 0x7e {
		t.Fatalf("期望保留末尾7e，实际%v", rest)
	}
	want := []string{"013924072039", "013901621187", "013901620325", "013921033017"}
	for i, f := range frames {
		if f[0] != 0x7e || f[len(f)-1] != 0x7e {
			t.Fatalf("帧%d首尾标识位错误", i+1)
		}
		if got := hex.EncodeToString(f[5:11]); got != want[i] {
			t.Fatalf("帧%d终端号=%s，期望%s", i+1, got, want[i])
		}
	}
}

// 半帧先到、后半段下次Read补齐，应无损合并
func TestExtractFramesPartial(t *testing.T) {
	data := mustHex(t, rawPacket)
	// 帧1完整127字节 + 帧2前73字节
	frames, rest := extractFrames(data[:200])
	if len(frames) != 1 {
		t.Fatalf("第一次期望1帧，实际%d帧", len(frames))
	}
	// 半帧与后续数据合并
	merged := append(append([]byte{}, rest...), data[200:]...)
	frames, rest = extractFrames(merged)
	if len(frames) != 3 {
		t.Fatalf("补齐后期望3帧，实际%d帧", len(frames))
	}
	if len(rest) != 1 || rest[0] != 0x7e {
		t.Fatalf("期望保留末尾7e，实际%v", rest)
	}
}

// 垃圾数据、空帧、共用标识位
func TestExtractFramesGarbage(t *testing.T) {
	// 无任何0x7E：纯垃圾，全部丢弃
	frames, rest := extractFrames([]byte{0x01, 0x02, 0x03})
	if frames != nil || rest != nil {
		t.Fatalf("纯垃圾应全部丢弃：frames=%v rest=%v", frames, rest)
	}
	// 前置垃圾 + 半帧头：保留7e及后续内容等待补齐
	frames, rest = extractFrames([]byte{0xaa, 0x7e, 0x02, 0x00})
	if len(frames) != 0 || len(rest) != 3 || rest[0] != 0x7e {
		t.Fatalf("半帧保留错误：frames=%d rest=%v", len(frames), rest)
	}
	// 连续空帧（7e7e7e7e）：跳过空帧，最后一个7e留作下一帧头
	frames, rest = extractFrames([]byte{0x7e, 0x7e, 0x7e, 0x7e})
	if len(frames) != 0 || len(rest) != 1 || rest[0] != 0x7e {
		t.Fatalf("空帧处理错误：frames=%d rest=%v", len(frames), rest)
	}
	// 共用标识位写法：7e 帧1内容 7e 帧2内容 7e
	frames, _ = extractFrames(mustHex(t, "7e0102000a0001234567890001303132333435363738397e0102000a0001234567890002303132333435363738397e"))
	if len(frames) != 2 {
		t.Fatalf("共用标识位应切出2帧，实际%d帧", len(frames))
	}
}

func TestTryHexText(t *testing.T) {
	if b, ok := tryHexText([]byte("7e0102000a")); !ok || hex.EncodeToString(b) != "7e0102000a" {
		t.Fatalf("hex文本未识别: ok=%v b=%v", ok, b)
	}
	if b, ok := tryHexText([]byte("0X7E0102")); !ok || hex.EncodeToString(b) != "7e0102" {
		t.Fatalf("0x前缀hex文本未识别: ok=%v b=%v", ok, b)
	}
	if _, ok := tryHexText([]byte{0x02, 0x00, 0x44, 0xff}); ok {
		t.Fatal("二进制数据被误识别为hex文本")
	}
}

// ---- 端到端 ----

type recordSink struct {
	mu   sync.Mutex
	locs []protocol.Location
}

func (r *recordSink) Push(loc *protocol.Location) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.locs = append(r.locs, *loc)
	return nil
}

func (r *recordSink) snapshot() []protocol.Location {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]protocol.Location, len(r.locs))
	copy(out, r.locs)
	return out
}

func startConn(t *testing.T, snk sink.MessageSink) net.Conn {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		NewConnection(c, snk).Run()
	}()
	dev, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dev.Close() })
	return dev
}

// drainReplies 持续读取应答直到超时，返回累计的hex串
func drainReplies(dev net.Conn, wait time.Duration) string {
	var sb strings.Builder
	buf := make([]byte, 8192)
	_ = dev.SetReadDeadline(time.Now().Add(wait))
	for {
		n, err := dev.Read(buf)
		if n > 0 {
			sb.WriteString(hex.EncodeToString(buf[:n]))
		}
		if err != nil {
			break
		}
	}
	return sb.String()
}

// 场景A：4帧一次写入（粘包）——修复前只入库1条且帧1数据被污染
func TestEndToEndCoalescedFrames(t *testing.T) {
	snk := &recordSink{}
	dev := startConn(t, snk)
	raw := mustHex(t, rawPacket)
	if _, err := dev.Write(raw); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	locs := snk.snapshot()
	if len(locs) != 4 {
		t.Fatalf("粘包4帧应全部入库4条，实际%d条", len(locs))
	}
	want := []string{
		"00000000013924072039",
		"00000000013901621187",
		"00000000013901620325",
		"00000000013921033017",
	}
	for i, loc := range locs {
		if loc.ImeiNum != want[i] {
			t.Fatalf("第%d条 imei=%s，期望%s", i+1, loc.ImeiNum, want[i])
		}
	}
	// 帧1附加信息不含fc/fd：举升/闭合必须为0（修复前会吃进后续帧字节解析出垃圾值）
	if locs[0].Lift != 0 || locs[0].Close != 0 {
		t.Fatalf("帧1被后续帧污染：lift=%d close=%d（均应为0）", locs[0].Lift, locs[0].Close)
	}
	// 每帧都应有0x8001成功应答（应答流水号+应答ID+结果）
	replies := drainReplies(dev, 500*time.Millisecond)
	for _, serial := range []string{"e36b", "e36c", "e36d", "e36e"} {
		if !strings.Contains(replies, serial+"020000") {
			t.Fatalf("缺少对流水号%s的应答，应答流=%s", serial, replies)
		}
	}
}

// 场景B：TCP从帧中间拆开两次到达——修复前报"报文内容或者格式不对"且全部丢弃
func TestEndToEndSplitFrames(t *testing.T) {
	snk := &recordSink{}
	dev := startConn(t, snk)
	raw := mustHex(t, rawPacket)
	if _, err := dev.Write(raw[:200]); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond) // 确保服务器分两次Read
	if _, err := dev.Write(raw[200:]); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if got := len(snk.snapshot()); got != 4 {
		t.Fatalf("TCP拆分到达的4帧应全部入库4条，实际%d条", got)
	}
	// 四帧均应有应答
	replies := drainReplies(dev, 500*time.Millisecond)
	for _, serial := range []string{"e36b", "e36c", "e36d", "e36e"} {
		if !strings.Contains(replies, serial+"020000") {
			t.Fatalf("缺少对流水号%s的应答，应答流=%s", serial, replies)
		}
	}
}

// 场景C：连发100次×4帧=400帧——修复前只入库1条
func TestEndToEndBurst(t *testing.T) {
	snk := &recordSink{}
	dev := startConn(t, snk)
	raw := mustHex(t, rawPacket)
	// 后台持续读取应答，防止发送缓冲堆积
	go drainReplies(dev, 30*time.Second)
	for i := 0; i < 100; i++ {
		if _, err := dev.Write(raw); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(5 * time.Second)
	if got := len(snk.snapshot()); got != 400 {
		t.Fatalf("连发400帧应全部入库400条，实际%d条", got)
	}
}
