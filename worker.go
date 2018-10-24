package gpool

import "time"

type Worker struct {
	pool *Pool
	task chan f
	lastWorkTime time.Time
}

func (w *Worker) run() {
	go func() {
		for f := range w.task {
			if f == nil {
				w.pool.decRunningCount()
				return
			}
			f()
			w.pool.putWorker(w)
		}
	}()
}
