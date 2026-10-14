package server

import (
	"encoding/hex"
	"fmt"
	"jt808-gateway/config"
	"jt808-gateway/internal/protocol"
	"jt808-gateway/internal/sink"
	"net"
	"sync"
	"time"

	"github.com/gomodule/redigo/redis"
)

// # 单个设备连接对象（JT808会话）

const readTimeout = 3 * time.Minute

type Connection struct {
	conn       net.Conn
	terminalNo string
	sink       sink.MessageSink
	closeChan  chan struct{}
	writeMu    sync.Mutex // 新增写锁
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

	buf := make([]byte, 4096)
	for {
		select {
		case <-c.closeChan:
			return
		default:
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(readTimeout))
		n, err := c.conn.Read(buf)
		if err != nil {
			return // 直接退出，触发defer关闭conn
		}
		//// %X 以大写、空格分隔的十六进制打印，如：7E 02 00 40 44 ... 7E，方便对照协议排查
		fmt.Printf("Received message from %s (%d bytes): %s\n", c.conn.RemoteAddr().String(), n, buf[:n])
		go c.handleFrame(buf[:n])
	}
}

func (c *Connection) handleFrame(messageBytes []byte) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("handleFrame panic, terminal:%s err=%v\n", c.terminalNo, r)
		}
	}()

	// 适配三种上送格式：
	//   1. 原始二进制（首字节为 0x7E，ASCII '~'）→ 转hex字符串
	//   2. hex文本带0x前缀（"0x7E02...7E"）     → 直接作为字符串
	//   3. hex文本无前缀（"7E02...7E"）           → 直接作为字符串
	// 判断依据：首字节是否为 0x7E。hex文本首字节是 '0'(0x30) 或 '7'(0x37)，不会是 0x7E
	var message string
	if len(messageBytes) > 0 && messageBytes[0] == 0x7E {
		message = hex.EncodeToString(messageBytes)
	} else {
		message = string(messageBytes)
	}
	msgheadPaket, msgBodyHexa, err := protocol.ParsePaketMessageId(message)
	if err != nil {
		fmt.Printf("handleFrame error, terminal:%s err=%v\n", c.conn.RemoteAddr().String(), err)
		_ = c.send([]byte("This is not a valid message format"))
		return
	}

	msgId := "0x" + msgheadPaket.MessageId
	switch msgId {
	case protocol.T0200:
		loc, err := protocol.GetLocation(msgBodyHexa)
		if err != nil {
			resp := protocol.CommonReply(msgheadPaket, "2")
			_ = c.send([]byte(resp))
			//_ = c.sendHex(resp)
			return
		}
		// 判断定位是否在库里
		exist, err := redis.Bool(config.RedisConn.Do("SISMEMBER", config.SetKey, loc.ImeiNum))
		if !exist {
			fmt.Println("ImeiNum not found in System:", loc.ImeiNum)
			resp := protocol.CommonReply(msgheadPaket, "3")
			//_ = c.sendHex(resp)
			_ = c.send([]byte(resp))
			return
		}
		_ = c.sink.Push(&loc)
		resp := protocol.CommonReply(msgheadPaket, "0")
		_ = c.send([]byte(resp))
		// _ = c.sendHex(resp)
		return
	case protocol.T0002:
		// 终端心跳，预留
		resp := protocol.CommonReply(msgheadPaket, "0")
		_ = c.send([]byte(resp))
		// _ = c.sendHex(resp)
		return
	default:
		resp := protocol.CommonReply(msgheadPaket, "1")
		_ = c.send([]byte(resp))
		// _ = c.sendHex(resp)
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
