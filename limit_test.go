package main

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const n = 100

func TestNewLimit(t *testing.T) {
	//TODO: use testing/synctest
	l := NewLimit()
	// 测试是否允许默认最大同时请求
	for range 300 * runtime.GOMAXPROCS(0) {
		if !l.Allow() {
			t.Fatalf("拒绝了本该允许的请求")
		}
	}
	// 测试是否拒绝超出限制的请求
	if l.Allow() {
		t.Fatalf("允许了超过限制的请求")
	}

	// 测试Allow实现是否存在竟态条件
	for range n*runtime.GOMAXPROCS(0) - 1 {
		if l.Allow() {
			t.Fatalf("允许了超过限制的请求")
		}
	}
	var wg sync.WaitGroup
	total := n * runtime.GOMAXPROCS(0)
	var fail atomic.Int64
	for range n * runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			l.End()
		})
		wg.Go(func() {
			if !l.Allow() {
				fail.Add(1)
			}
		})
	}
	t.Log(total, fail.Load())
	if int(fail.Load()) > total/2 {
		t.Fatalf("Allow实现存在竟态条件")
	}
	//TODO: 更精确的测试
	time.Sleep(2 * time.Second)
	if !l.Allow() {
		t.Fatalf("got false, want true")
	}
}

func BenchmarkLimit(b *testing.B) {
	l := NewLimit()
	b.Run("1", func(b *testing.B) {
		for b.Loop() {
			l.Allow()
			l.End()
		}
	})
	b.Run("p", func(b *testing.B) {
		b.RunParallel(func(p *testing.PB) {
			for p.Next() {
				l.Allow()
				l.End()
			}
		})
	})
}
