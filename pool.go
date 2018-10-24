package gpool

import (
	"time"
	"sync"
	"fmt"
	"sync/atomic"
)

type f func() error


type Pool struct {
	maxWorkerCount int32

	runningWorkerCount int32

	maxWorkerIdleDuration time.Duration

	ticker *time.Ticker

	freeWorker []*Worker

	release chan struct{}

	lock sync.Mutex

	cond *sync.Cond

	once sync.Once
}

func NewPool(maxWorkerCount int) (*Pool, error) {
	return NewPoolWithIdle(maxWorkerCount, DefaultMaxWorkerIdleDuration)
}

func NewPoolWithIdle(maxWorkerCount, idle int) (*Pool, error) {
	if maxWorkerCount <= 0{
		return nil, fmt.Errorf("worker max count must greater than zero")
	}
	if idle <= 0 {
		return nil, fmt.Errorf("worker max idle time must greater than zero")
	}

	p := &Pool{
		maxWorkerCount: int32(maxWorkerCount),
		release: make(chan struct{}, 1),
		maxWorkerIdleDuration: time.Duration(idle) * time.Second,
	}
	p.cond = sync.NewCond(&p.lock)
	go p.clean()
	return p, nil
}

func (p *Pool) Close() {
	p.once.Do(func() {
		p.release <- struct{}{}
		p.ticker.Stop()
		p.lock.Lock()
		for i, w := range p.freeWorker {
			w.task <- nil
			p.freeWorker[i] = nil
		}
		p.freeWorker = nil
		p.lock.Unlock()
	})
}

func (p *Pool)Submit(task f) error {
	if len(p.release) > 0 {
		return fmt.Errorf("poll has closed")
	}
	p.getWorker().task <- task
	return nil
}

func (p *Pool) getWorker() *Worker {
	var worker *Worker
	wait := false
	p.lock.Lock()
	defer p.lock.Unlock()
	freeWorker := p.freeWorker
	n := len(freeWorker)
	if n > 0 {
		worker = freeWorker[n - 1]
		p.freeWorker = freeWorker[:n - 1]
		return worker
	} else {
		if p.runningWorkerCount >= p.maxWorkerCount {
			wait = true
		}
	}

	if wait {
		for {
			p.cond.Wait()
			freeWorker := p.freeWorker
			l := len(freeWorker) - 1
			if l < 1 {
				continue
			}

			worker = freeWorker[l]
			freeWorker[l] = nil
			p.freeWorker = freeWorker[:l]
			break
		}
	} else {
		worker = &Worker{
			pool: p,
			task: make(chan f, 1),
		}
		worker.run()
		p.incRunningCount()
	}

	return worker
}

func (p *Pool) putWorker(worker *Worker) {
	if len(p.release) > 0 {
		return
	}
	worker.lastWorkTime = time.Now()
	p.lock.Lock()
	defer p.lock.Unlock()
	p.freeWorker = append(p.freeWorker, worker)
	p.cond.Signal()
}

func (p *Pool) Resize(maxWorkerCount int) error {
	if maxWorkerCount < 0 {
		return fmt.Errorf("worker max count must greater then zero")
	}

	if maxWorkerCount == p.MaxWorkerCount() {
		return nil
	}
	diff := p.MaxWorkerCount() - maxWorkerCount

	for i:=0; i < diff; i++ {
		p.getWorker().task <- nil
	}
	return nil
}

func (p *Pool) ResetWorkerIdle(idle int) error {
	if idle < 0 {
		return fmt.Errorf("worker idle must greater then zero")
	}
	if idle == p.MaxWorkerIdle() {
		return nil
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	p.maxWorkerIdleDuration = time.Duration(idle) * time.Second
	return nil
}

func (p *Pool)incRunningCount() {
	atomic.AddInt32(&p.runningWorkerCount, 1)
}

func (p *Pool)decRunningCount() {
	atomic.AddInt32(&p.runningWorkerCount, -1)
}

func (p *Pool) RunningCount() int {
	return int(atomic.LoadInt32(&p.runningWorkerCount))
}

func (p *Pool) MaxWorkerCount() int {
	return int(atomic.LoadInt32(&p.maxWorkerCount))
}

func (p *Pool) MaxWorkerIdle() int {
	p.lock.Lock()
	duration := p.maxWorkerIdleDuration
	p.lock.Unlock()
	return int(duration/time.Second)
}

func (p *Pool) clean() {
	p.ticker = time.NewTicker(p.maxWorkerIdleDuration)

	for range p.ticker.C {
		current := time.Now()
		p.lock.Lock()
		n := len(p.freeWorker)
		if  n == 0 {
			p.lock.Unlock()
			return
		}
		j := -1
		for i := 0;i < n; i ++ {
			if current.Sub(p.freeWorker[i].lastWorkTime) < p.maxWorkerIdleDuration {
				break
			}
			p.freeWorker[i].task <- nil
			p.freeWorker[i] = nil
			j = i
		}

		if j > -1 {
			if j >= n {
				p.freeWorker = p.freeWorker[:0]
			} else {
				p.freeWorker = p.freeWorker[j+1:]
			}
		}
		p.lock.Unlock()
	}
}