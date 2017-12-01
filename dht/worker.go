package dht

import (
	"sync"
)

type WorkerJob func() chan interface{}
type WorkerResult interface{}

type WorkerQueue struct {
	sync.RWMutex
	size    int
	waiting int
	running bool
	jobs    chan WorkerJob
	Results chan WorkerResult
}

func NewWorkerQueue(size, buffer int) *WorkerQueue {
	return &WorkerQueue{
		running: false,
		size:    size,
		jobs:    make(chan WorkerJob, buffer),
		Results: make(chan WorkerResult, buffer),
	}
}

func (this *WorkerQueue) worker(id int) {
	for j := range this.jobs {
		val, _ := <-j()
		// if !ok {
		// 	return
		// }
		this.Results <- val
	}
	// this.onDone()
}

func (this *WorkerQueue) OnDone() {
	this.Lock()

	this.waiting--

	if this.waiting == 0 {
		this.Unlock()
		this.Stop()
	} else {
		this.Unlock()
	}
}

func (this *WorkerQueue) Start() {
	if this.IsRunning() {
		return
	}

	this.Lock()
	this.running = true
	this.Unlock()

	for w := 1; w <= this.size; w++ {
		go this.worker(w)
	}
}
func (this *WorkerQueue) Stop() {
	if !this.IsRunning() {
		return
	}

	this.Lock()
	this.running = false
	this.Unlock()

	close(this.jobs)
	close(this.Results)
}

func (this *WorkerQueue) Add(job WorkerJob) {
	this.Lock()

	if !this.running {
		this.Unlock()
		return
	}

	this.waiting++

	this.jobs <- job
	this.Unlock()
}

func (this *WorkerQueue) IsRunning() bool {
	this.RLock()
	defer this.RUnlock()

	return this.running
}

func (this *WorkerQueue) WaitingCount() int {
	this.RLock()
	defer this.RUnlock()

	return this.waiting
}
