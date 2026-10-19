package server

// # JT808帧解析：TCP字节流按0x7E分帧、半帧缓冲、hex文本兼容

import (
	"bytes"
	"encoding/hex"
	"strings"
)

// maxFrameSize 单帧最大缓存长度（含转义；JT808消息体长度上限1023字节，此处留足余量）。
// 迟迟等不到帧尾0x7E时按脏数据丢弃，防止缓冲无限增长。
const maxFrameSize = 32 * 1024

// extractFrames 从接收缓冲中切出全部完整帧（含首尾0x7E，独立拷贝）。
// 转义后的帧内不会出现裸0x7E（会被转成7d02），因此按0x7E字节定位帧边界是安全的；
// 相邻两个0x7E之间有内容即为一帧，同时兼容"帧间7e7e双标识位"与"共用标识位"两种写法。
// 返回值rest为剩余的不完整数据（可能只有一个0x7E，等待后续Read补齐）。
func extractFrames(data []byte) ([][]byte, []byte) {
	var frames [][]byte
	pos := bytes.IndexByte(data, 0x7e)
	if pos < 0 {
		// 无标识位：纯垃圾数据，全部丢弃等待重同步
		return nil, nil
	}
	for {
		next := bytes.IndexByte(data[pos+1:], 0x7e)
		if next < 0 {
			rest := data[pos:]
			if len(rest) > maxFrameSize {
				// 半帧超长：视为脏数据丢弃重新同步
				rest = nil
			}
			return frames, rest
		}
		next += pos + 1
		if next > pos+1 { // 两个0x7E之间存在内容 → 一帧
			frame := make([]byte, next-pos+1)
			copy(frame, data[pos:next+1])
			frames = append(frames, frame)
		}
		pos = next // 帧尾0x7E同时可作为下一帧的帧头
	}
}

// tryHexText 判断读到的数据是否为hex文本上行（如"7e0200..."或"0x7e..."），是则解码为字节。
// 仅当整段都由hex字符构成时才转换，避免误伤二进制数据；
// 只应在接收缓冲为空（无二进制半帧在途）时调用。
func tryHexText(chunk []byte) ([]byte, bool) {
	s := strings.TrimSpace(string(chunk))
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	s = strings.NewReplacer(" ", "", "\r", "", "\n", "", "\t", "").Replace(s)
	if len(s) < 4 || len(s)%2 != 0 {
		return nil, false
	}
	for i := 0; i < len(s); i++ {
		if !isHexDigit(s[i]) {
			return nil, false
		}
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, false
	}
	return b, true
}

func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}
