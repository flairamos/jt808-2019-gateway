package server

import (
	"encoding/hex"
	"fmt"
	"jt808-gateway/internal/protocol"
	"jt808-gateway/internal/sink"
	"net"
	"os"
	"sync"
	"time"
)

// # 单个设备连接对象（JT808会话）

const readTimeout = 3 * time.Minute

// rawLogEnabled 控制是否打印每次Read的十六进制原样内容（排障用，默认开启）。
// 压测或大流量场景可设环境变量 JT808_RAWLOG=0 关闭，避免逐帧日志IO成为吞吐瓶颈。
var rawLogEnabled = os.Getenv("JT808_RAWLOG") != "0"

type Connection struct {
	conn       net.Conn
	terminalNo string
	sink       sink.MessageSink
	closeChan  chan struct{}
	writeMu    sync.Mutex // 写锁
}

func NewConnection(conn net.Conn, sink sink.MessageSink) *Connection {
	return &Connection{
		conn:      conn,
		sink:      sink,
		closeChan: make(chan struct{}),
	}
}

func (c *Connection) Run() {
	defer func() {
		c.Close() // goroutine退出必然关闭连接
		fmt.Println(c.conn.RemoteAddr().String(), c.conn.LocalAddr().String(), "release connection")
	}()

	// 打印新connection的remote addr
	fmt.Println(c.conn.RemoteAddr().String(), "accept new connection")

	readBuf := make([]byte, 4096)
	// 跨Read的接收缓冲：TCP是字节流，一次Read可能读到半帧或多帧（聚合设备一条连接带多台车，报文经常粘连），
	// 必须先累积再按0x7E分帧，半帧留在缓冲等下一次Read补齐。
	// 初始容量512按需增长：长连接多为空闲，可显著降低单连接内存占用
	rxBuf := make([]byte, 0, 512)
	for {
		select {
		case <-c.closeChan:
			return
		default:
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(readTimeout))
		n, err := c.conn.Read(readBuf)
		if n > 0 {
			if rawLogEnabled {
				fmt.Println(c.conn.RemoteAddr().String(), hex.EncodeToString(readBuf[:n]))
			}
			chunk := readBuf[:n]
			// 兼容hex文本上行的设备：接收缓冲为空且非0x7E开头时尝试按hex文本解码
			// （仅在无半帧在途时尝试，避免误伤二进制半帧续传）
			if chunk[0] != 0x7E && len(rxBuf) == 0 {
				if decoded, ok := tryHexText(chunk); ok {
					chunk = decoded
				}
			}
			rxBuf = append(rxBuf, chunk...)
			frames, rest := extractFrames(rxBuf)
			rxBuf = append(rxBuf[:0], rest...)
			for _, frame := range frames {
				// 帧为独立拷贝（不与readBuf/rxBuf共享底层数组）；同步处理保证处理与应答顺序
				c.handleFrame(frame)
			}
		}
		if err != nil {
			// n>0且err!=nil时先处理完本次数据再退出
			return
		}
	}
}

func (c *Connection) handleFrame(frame []byte) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("handleFrame panic, terminal:%s err=%v\n", c.conn.RemoteAddr().String(), r)
		}
	}()

	// frame为已分帧、独立拷贝的完整帧（含首尾0x7E）
	var message string
	if len(frame) > 0 && frame[0] == 0x7E {
		message = hex.EncodeToString(frame)
	} else {
		message = string(frame)
	}
	// ParsePaketMessageIdAuto 自动识别2011/2013/2019版本，并完成转义还原、校验码与消息体长度校验
	msgheadPaket, process_message, err := protocol.ParsePaketMessageIdAuto(message)
	if err != nil {
		// 帧损坏（校验码/长度不符等）：记录原始帧便于排查，直接丢弃。
		// 不回文本应答：设备走二进制协议，且ASCII文本经sendHex的hex解码必然失败，设备什么都收不到
		fmt.Printf("handleFrame error, terminal:%s err=%v frame=%s\n", c.conn.RemoteAddr().String(), err, message)
		return
	}
	// 分包/加密报文暂不支持：分帧后单帧无法还原完整消息体，按0x8001结果码3明确应答，避免设备无限重发
	if msgheadPaket.IsSubpackaged() || msgheadPaket.IsEncrypted() {
		fmt.Printf("handleFrame unsupported, terminal:%s phone:%s subpackaged=%v encrypted=%v\n",
			c.conn.RemoteAddr().String(), msgheadPaket.TerminalPhone, msgheadPaket.IsSubpackaged(), msgheadPaket.IsEncrypted())
		_ = c.sendHex(protocol.CommonReplyAuto(msgheadPaket, "3"))
		return
	}
	msgId := "0x" + msgheadPaket.MessageId
	switch msgId {
	case protocol.T0200:
		// 位置信息汇报
		loc, err := protocol.GetLocationAuto(msgheadPaket, process_message)
		if err != nil {
			_ = c.sendHex(protocol.CommonReplyAuto(msgheadPaket, "2"))
			return
		}

		// 判断定位是否在库里
		//exist, err := redis.Bool(redisutil.RedisConn.Do("SISMEMBER", variables.SetKey, loc.ImeiNum))
		//if !exist {
		//	fmt.Println("ImeiNum not found in System:", loc.ImeiNum)
		//	resp := protocol.CommonReplyAuto(msgheadPaket, "3")
		//	_ = c.sendHex(resp)
		//	return
		//}
		_ = c.sink.Push(&loc)
		_ = c.sendHex(protocol.CommonReplyAuto(msgheadPaket, "0"))
		return
	case protocol.T0704:
		// 批量定位数据上报：一个数据项就是一条0x0200消息体，逐条入库
		locs, err := protocol.GetLocationBatchAuto(msgheadPaket, process_message)
		if err != nil {
			_ = c.sendHex(protocol.CommonReplyAuto(msgheadPaket, "2"))
			return
		}
		for i := range locs {
			_ = c.sink.Push(&locs[i])
		}
		_ = c.sendHex(protocol.CommonReplyAuto(msgheadPaket, "0"))
		return
	case protocol.T0001:
		// 终端通用应答（设备确认平台下行指令），无需再回复
		return
	case protocol.T0002:
		// 终端心跳
		_ = c.sendHex(protocol.CommonReplyAuto(msgheadPaket, "0"))
		return
	case protocol.T0102:
		// 终端鉴权
		_ = c.sendHex(protocol.CommonReplyAuto(msgheadPaket, "0"))
		return
	case protocol.T0100:
		// 终端注册
		// 车辆首次安装终端上电、更换终端设备、终端恢复出厂设置
		// 此处可解析消息体存储设备信息并生成校验码
		_ = c.sendHex(protocol.RegisterReply(msgheadPaket, "0"))
		return
	default:
		_ = c.sendHex(protocol.CommonReplyAuto(msgheadPaket, "1"))
		return
	}
}

// sendHex 将hex字符串解码为二进制后发送（设备收的是原始二进制，不是hex文本）
func (c *Connection) sendHex(hexStr string) error {
	raw, err := hex.DecodeString(hexStr)
	if err != nil {
		return err
	}
	return c.send(raw)
}

// 安全发送下行数据，加锁+写超时
func (c *Connection) send(raw []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
	_, err := c.conn.Write(raw)
	return err
}

func (c *Connection) Close() {
	close(c.closeChan)
	_ = c.conn.Close()
}
