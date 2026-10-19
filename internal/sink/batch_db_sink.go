package sink

import (
	"context"
	"errors"
	"fmt"
	"jt808-gateway/internal/protocol"
	"time"

	"gorm.io/gorm"
)

// # 实现：直接批量入库（试点模式）

const (
	batchMaxSize  = 500              // 最大批量条数
	flushInterval = 15 * time.Second // 定时刷新
)

type PgSink struct {
	pool *gorm.DB
	ch   chan *protocol.Location
	ctx  context.Context
}

func NewPgSink(ctx context.Context, pool *gorm.DB) *PgSink {
	s := &PgSink{
		pool: pool,
		ch:   make(chan *protocol.Location, 2000),
		ctx:  ctx,
	}
	go s.batchLoop()
	return s
}

// 数据入队
func (s *PgSink) Push(loc *protocol.Location) error {
	select {
	case s.ch <- loc:
	default:
		// 队列满，丢弃旧定位（定位时效性极强）
		<-s.ch
		s.ch <- loc
	}
	return nil
}

func (s *PgSink) batchLoop() {
	// 初始化缓冲区，预分配容量，避免频繁扩容
	buf := make([]*protocol.Location, 0, batchMaxSize)
	// 定时触发器，到达时间强制刷新缓冲区
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop() // 函数退出务必关闭定时器，防止goroutine泄漏

	for { // 永久消费循环，直到上下文取消
		select {
		case <-s.ctx.Done(): // 分支1：收到退出信号（服务关闭、优雅停机）
			if len(buf) > 0 {
				s.flush(buf) // 缓冲区剩余数据强制落库，防止丢数据
			}
			return // 退出循环，消费结束
		case item := <-s.ch: // 分支2：上游channel收到一条定位数据
			buf = append(buf, item)
			if len(buf) >= batchMaxSize { // 判断缓冲区达到最大批量阈值，立刻执行刷新
				s.flush(buf)
				buf = buf[:0] // 切片清空，复用底层数组，减少GC
			}
		case <-ticker.C: // 分支3：定时触发，避免消息稀疏时数据长期滞留内存
			if len(buf) > 0 {
				s.flush(buf)
				buf = buf[:0]
			}
		}
	}
}

func (s *PgSink) flush(list []*protocol.Location) {
	ctx, cancel := context.WithTimeout(s.ctx, 3*time.Second)
	defer cancel()
	if len(list) == 0 {
		return
	}
	err := s.pool.WithContext(ctx).Table("iot_car_location").Create(&list).Error
	if err != nil {
		// 入库失败必须如实记录：本批数据已丢失，不能继续打印成功日志误导排查
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			fmt.Printf("flush track batch timeout, drop %d records: %v\n", len(list), err)
		} else {
			fmt.Printf("flush track batch failed, drop %d records: %v\n", len(list), err)
		}
		return
	}
	fmt.Printf("flush track batch size: %d\n", len(list))
}
