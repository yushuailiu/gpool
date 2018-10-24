package gpool

import (
	"testing"
	"sync"
	"time"
	"runtime"
	"math"
)

const (
	_   = 1 << (10 * iota)
	KiB // 1024
	MiB // 1048576
	n = 100000000
)
var curMem uint64

func sleepFunc() error {
	time.Sleep(time.Millisecond * 10)
	return nil
}

func TestGpool(t *testing.T) {
	pool,err := NewPool(math.MaxInt32)
	if err != nil {
		t.Error("create new pool fail")
	}
	var wg sync.WaitGroup
	for i:= 0; i< n; i++ {
		wg.Add(1)
		pool.Submit(func() error {
			sleepFunc()
			wg.Done()
			return nil
		})
	}
	wg.Wait()

	t.Logf("max worker count of pool:%d", pool.MaxWorkerCount())
	t.Logf("max worker idle of pool:%d", pool.MaxWorkerIdle())
	t.Logf("running worker count of pool:%d", pool.RunningCount())

	mem := runtime.MemStats{}
	runtime.ReadMemStats(&mem)
	curMem = mem.TotalAlloc/MiB - curMem
	t.Logf("memory usage:%d MB", curMem)
}

func TestNoPool(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() error {
			sleepFunc()
			wg.Done()
			return nil
		}()
	}
	wg.Wait()
	mem := runtime.MemStats{}
	runtime.ReadMemStats(&mem)
	curMem = mem.TotalAlloc/MiB - curMem
	t.Logf("memory usage:%d MB", curMem)
}