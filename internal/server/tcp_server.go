package server

import (
	"context"
	"fmt"
	"jt808-gateway/internal/sink"
	"net"
	"sync"
)

//# TCP监听，接受设备连接

type TcpServer struct {
	listener net.Listener
	addr     string
	sink     sink.MessageSink
	wg       sync.WaitGroup
	ctx      context.Context
}

func NewTcpServer(ctx context.Context, addr string, sink sink.MessageSink) *TcpServer {
	return &TcpServer{
		addr: addr,
		sink: sink,
		ctx:  ctx,
	}
}

func (s *TcpServer) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.listener = listener
	fmt.Printf("JT808 TCP server listen on %s\n", s.addr)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-s.ctx.Done():
					return
				default:
					fmt.Printf("accept err: %v\n", err)
					return
				}
			}
			clientConn := NewConnection(conn, s.sink)
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				clientConn.Run()
			}()
		}
	}()
	return nil
}

func (s *TcpServer) Stop() {
	_ = s.listener.Close()
	s.wg.Wait()
	fmt.Println("tcp server gracefully stopped")
}
