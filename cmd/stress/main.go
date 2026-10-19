package main

// JT808网关压测工具（用真实 TcpServer + 内存计数sink，不依赖DB/Redis）：
//   server  模式：起服务，每5秒输出 goroutine数/内存/已入库定位数
//   client  模式：并发建连长连接、按间隔发送真实定位帧、统计发送/应答/存活
//   latency 模式：单连接逐帧测 发送→0x8001应答 的往返延迟分位数
//
// 用法：
//   stress -mode server -ports 7101
//   stress -mode client -addr 127.0.0.1 -ports 7101 -conns 10000 -duration 30s
//   stress -mode client -addr 127.0.0.1 -ports 7101 -conns 500 -interval 50ms -duration 20s   (吞吐)
//   stress -mode latency -addr 127.0.0.1 -ports 7101

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"jt808-gateway/internal/protocol"
	"jt808-gateway/internal/server"
)

// 现场抓包中合法的0x0200定位帧（127字节，含校验码），压测时循环发送
const sampleFrame = "7e02000070013924072039e36b00000000000800030124e5e6067f9652006f0104011b2610151601050104000b8d5d0202000003020118140480000000150400000000160400000000170200002504000000002b040000000030011d31010eeb0d000300d400000600f880000000ef0d00000049240049249200001103587e"

// countingSink 只统计入库条数，用于隔离网络/协议层性能
type countingSink struct{ locations atomic.Int64 }

func (s *countingSink) Push(loc *protocol.Location) error {
	s.locations.Add(1)
	return nil
}

func main() {
	mode := flag.String("mode", "", "server | client | latency")
	addr := flag.String("addr", "127.0.0.1", "server:监听IP；client/latency:目标IP")
	ports := flag.String("ports", "7101", "逗号分隔端口：server依次监听，client轮询连接")
	conns := flag.Int("conns", 1000, "client: 总连接数")
	interval := flag.Duration("interval", 0, "client: 每连接发送间隔，0=连接后只发一帧(容量测试)")
	duration := flag.Duration("duration", 30*time.Second, "client: 建连完成后的保持时长")
	ramp := flag.Int("ramp", 256, "client: 并发建连数")
	flag.Parse()

	switch *mode {
	case "server":
		runServer(*addr, splitPorts(*ports))
	case "client":
		runClient(*addr, splitPorts(*ports), *conns, *interval, *duration, *ramp)
	case "latency":
		runLatency(*addr, splitPorts(*ports)[0])
	default:
		flag.Usage()
		os.Exit(1)
	}
}

func splitPorts(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func runServer(addr string, ports []string) {
	ctx := context.Background()
	snk := &countingSink{}
	for _, p := range ports {
		if err := server.NewTcpServer(ctx, addr+":"+p, snk).Start(); err != nil {
			fmt.Fprintln(os.Stderr, "listen failed:", addr+":"+p, err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "listening on", addr+":"+p)
	}
	var ms runtime.MemStats
	for {
		time.Sleep(5 * time.Second)
		runtime.ReadMemStats(&ms)
		fmt.Fprintf(os.Stderr, "[%s][server] goroutines=%d heap=%.1fMB sys=%.1fMB locations=%d\n",
			time.Now().Format("15:04:05"), runtime.NumGoroutine(),
			float64(ms.HeapAlloc)/1e6, float64(ms.Sys)/1e6, snk.locations.Load())
	}
}

type clientStats struct {
	live, dead, dialFail, sent atomic.Int64
	replySevens                atomic.Int64 // 收到的0x7E字节数(每条应答恰含首尾2个)
}

func (st *clientStats) replies() int64 { return st.replySevens.Load() / 2 }

func runClient(addr string, ports []string, total int, interval time.Duration, hold time.Duration, ramp int) {
	frame, err := hex.DecodeString(sampleFrame)
	if err != nil {
		panic(err)
	}
	st := &clientStats{}
	stopCh := make(chan struct{})
	var wg sync.WaitGroup
	sem := make(chan struct{}, ramp)
	start := time.Now()

	go func() { // 周期进度
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for range t.C {
			select {
			case <-stopCh:
				return
			default:
			}
			fmt.Printf("[%s][client] 存活=%d 建连失败=%d 断开=%d 已发送=%d 应答帧=%d 用时=%.0fs\n",
				time.Now().Format("15:04:05"), st.live.Load(), st.dialFail.Load(), st.dead.Load(),
				st.sent.Load(), st.replies(), time.Since(start).Seconds())
		}
	}()

	for i := 0; i < total; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			runConn(net.JoinHostPort(addr, ports[i%len(ports)]), frame, interval, stopCh, st)
		}(i)
	}

	// 等全部连接到达终态（建立成功或失败）
	for st.live.Load()+st.dead.Load()+st.dialFail.Load() < int64(total) {
		time.Sleep(200 * time.Millisecond)
	}
	rampTime := time.Since(start)
	fmt.Printf("[%s][client] 建连完成: 存活=%d 失败=%d 断开=%d 建连耗时=%.1fs，保持%v\n",
		time.Now().Format("15:04:05"), st.live.Load(), st.dialFail.Load(), st.dead.Load(), rampTime.Seconds(), hold)
	time.Sleep(hold)
	close(stopCh)
	wg.Wait()
	time.Sleep(300 * time.Millisecond)
	fmt.Printf("[%s][client] 结束: 目标=%d 存活=%d 建连失败=%d 断开=%d 发送帧=%d 应答帧=%d 建连耗时=%.1fs\n",
		time.Now().Format("15:04:05"), total, st.live.Load(), st.dialFail.Load(), st.dead.Load(),
		st.sent.Load(), st.replies(), rampTime.Seconds())
}

func runConn(dst string, frame []byte, interval time.Duration, stopCh <-chan struct{}, st *clientStats) {
	conn, err := net.DialTimeout("tcp", dst, 10*time.Second)
	if err != nil {
		st.dialFail.Add(1)
		return
	}
	defer conn.Close()
	st.live.Add(1)
	defer st.live.Add(-1)

	// 读协程：排空应答并统计（唯一负责报告连接死亡的地方）
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				var sevens int64
				for _, b := range buf[:n] {
					if b == 0x7e {
						sevens++
					}
				}
				st.replySevens.Add(sevens)
			}
			if err != nil {
				st.dead.Add(1)
				return
			}
		}
	}()

	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write(frame); err != nil {
		return
	}
	st.sent.Add(1)
	if interval <= 0 {
		<-stopCh // 容量模式：发一帧后保持长连接
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-t.C:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := conn.Write(frame); err != nil {
				return
			}
			st.sent.Add(1)
		}
	}
}

func runLatency(addr, port string) {
	frame, _ := hex.DecodeString(sampleFrame)
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(addr, port), 5*time.Second)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	const N = 2000
	rtts := make([]time.Duration, 0, N)
	buf := make([]byte, 128)
	for i := 0; i < N; i++ {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		t0 := time.Now()
		if _, err := conn.Write(frame); err != nil {
			panic(err)
		}
		// 读到以0x7E结尾的完整应答（应答体内的0x7E已转义为7d02，不会出现裸0x7E）
		got := 0
		for {
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, err := conn.Read(buf[got:])
			if err != nil {
				panic(fmt.Sprintf("第%d帧未收到应答: %v", i+1, err))
			}
			got += n
			if got >= 20 && buf[got-1] == 0x7e {
				break
			}
		}
		rtts = append(rtts, time.Since(t0))
	}
	sort.Slice(rtts, func(i, j int) bool { return rtts[i] < rtts[j] })
	var total time.Duration
	for _, r := range rtts {
		total += r
	}
	fmt.Printf("[latency] n=%d avg=%v p50=%v p95=%v p99=%v max=%v\n",
		N, total/N, rtts[N/2], rtts[N*95/100], rtts[N*99/100], rtts[N-1])
}
