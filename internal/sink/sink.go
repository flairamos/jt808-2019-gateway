package sink

import "jt808-gateway/internal/protocol"

// # 消息输出抽象接口
type MessageSink interface {
	Push(loc *protocol.Location) error
}
