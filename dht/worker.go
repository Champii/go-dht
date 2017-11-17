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
	jobs    chan WorkerJob
	Results chan WorkerResult
}

func NewWorkerQueue(size, buffer int) *WorkerQueue {
	return &WorkerQueue{
		size:    size,
		jobs:    make(chan WorkerJob, buffer),
		Results: make(chan WorkerResult, buffer),
	}
}

func (this *WorkerQueue) worker(id int) {
	for j := range this.jobs {
		this.Results <- <-j()
		this.onDone()
	}
}

func (this *WorkerQueue) onDone() {
	this.Lock()
	defer this.Unlock()

	this.waiting--

	if this.waiting == 0 {
		close(this.jobs)
		close(this.Results)
	}
}

func (this *WorkerQueue) Start() {
	for w := 1; w <= this.size; w++ {
		go this.worker(w)
	}
}

func (this *WorkerQueue) Add(job WorkerJob) {
	this.Lock()
	this.waiting++
	this.Unlock()

	this.jobs <- job
}
