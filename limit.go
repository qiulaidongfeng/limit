package limit

import (
	"log/slog"
	"math"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

// Limit 实现基于可用资源的自适应限流。
//
// 实现是用一个动态的处理窗口，先用大窗口，最大限度利用服务器，
// 基于CPU使用率和剩余内存，判断是否接近最大处理能力，
// 快用完处理能力时快速缩小处理窗口，通过拒绝少部分过量请求，确保大多数请求成功处理。
//
// 和滑动窗口等其他限流算法比，好处在不限制每秒处理请求，能更大限度利用服务器.
//
// 目前实现
//
// 默认限制，同时最多处理300*runtime.GOMAXPROCS(0)个请求
//
// 基于CPU使用率
//
//   - 大于85，同时最多处理10*runtime.GOMAXPROCS(0)个请求
//   - 大于75，同时最多处理32*runtime.GOMAXPROCS(0)个请求
//   - 大于65，同时最多处理64*runtime.GOMAXPROCS(0)个请求
//
// 基于剩余内存
//
//   - 小于50Mb，同时最多处理10*runtime.GOMAXPROCS(0)个请求
//   - 小于100Mb，同时最多处理32*runtime.GOMAXPROCS(0)个请求
//   - 小于150Mb，同时最多处理64*runtime.GOMAXPROCS(0)个请求
//
// 所有限制中，选择最低的。
type Limit struct {
	// token 表示当前同时最多可处理的请求数。
	//TODO:rename
	token atomic.Int64

	//TODO: 考虑网络I/O。
}

// NewLimit 创建一个新的限流器。
// 创建的限流器不能被关闭。
func NewLimit() *Limit {
	r := &Limit{}
	r.token.Store(mulPerCpu(300))
	go r.adjust()
	return r
}

// Allow 报告是否可以处理新的1个请求。
func (l *Limit) Allow() bool {
	for {
		n := l.token.Load()
		if n <= 0 {
			return false
		}
		if l.token.CompareAndSwap(n, n-1) {
			return true
		}
	}
}

// End 表示处理完1个请求。
func (l *Limit) End() {
	l.token.Add(1)
}

// adjust 调整同时可处理的请求数。
func (l *Limit) adjust() {
	default_limit := mulPerCpu(300)
	next := default_limit
	for {
		next2 := default_limit
		tmp := getCPUUsage()
		if tmp > 85 {
			next2 = mulPerCpu(10)
		} else if tmp > 75 {
			next2 = mulPerCpu(32)
		} else if tmp > 65 {
			next2 = mulPerCpu(64)
		}
		ava := getMemoryAvailable()
		if ava/gb1 < 0.05 {
			next2 = min(next2, mulPerCpu(10))
		} else if ava/gb1 < 0.10 {
			next2 = min(next2, mulPerCpu(32))
		} else if ava/gb1 < 0.15 {
			next2 = min(next2, mulPerCpu(64))
		}
		l.token.Store(next2)
		if next != next2 {
			slog.Info("自适应限流器：", "之前：", next, "现在", next2)
		}
		next = next2
	}
}

// mulPerCpu 将参数乘最大可用逻辑CPU数。
func mulPerCpu(one int) int64 {
	return int64(one * runtime.GOMAXPROCS(0))
}

// getCPUUsage 获取CPU使用率(百分比)。
// 它会阻塞1秒钟多一点。
// 如果获取失败，返回1%。（相当于禁用基于它的限流）
func getCPUUsage() float64 {
	// 获取最近1秒的CPU使用率。
	percent, err := cpu.Percent(time.Second, false)
	if err != nil {
		slog.Error("", "err", err)
		return 1
	}
	if len(percent) == 0 {
		return 1
	}
	return percent[0]
}

// getMemoryAvailable 获取可用内存。
// 如果获取失败，返回无限可用内存。（相当于禁用基于它的限流）
func getMemoryAvailable() float64 {
	v, err := mem.VirtualMemory()
	if err != nil {
		slog.Error("", "err", err)
		return math.MaxInt64
	}
	return float64(v.Available)
}

var gb1 = float64(1024 * 1024 * 1024)
