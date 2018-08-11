package dht

import (
	"errors"
	"sort"
	"sync"
)

type QueryJob func(*Node) *Response

type Query struct {
	sync.RWMutex
	best        []*Node
	blacklist   []*Node
	hash        []byte
	closest     []byte
	job         QueryJob
	dht         *Dht
	workerQueue *WorkerQueue
	result      chan interface{}
}

func NewQuery(hash []byte, job QueryJob, dht *Dht) *Query {
	return &Query{
		best:        []*Node{},
		job:         job,
		dht:         dht,
		hash:        hash,
		closest:     dht.hash,
		workerQueue: NewWorkerQueue(7, 100),
		result:      make(chan interface{}, 1),
	}
}

func (this *Query) Run() *Response {
	bucket := this.dht.routing.FindNode(this.hash)

	if len(bucket) == 0 {
		return &Response{
			Err: errors.New("Not found"),
		}
	}

	this.workerQueue.Start()

	for _, node := range bucket {
		this.Lock()
		this.best = append(this.best, node)
		this.blacklist = append(this.blacklist, node)
		this.Unlock()

		this.updateClosest(node.Contact)
		this.addToQueue(node)
	}

	return this.WaitResult()
}

func (this *Query) updateClosest(contact PacketContact) {
	if this.isCloser(contact) {
		this.Lock()
		this.closest = contact.Hash
		this.Unlock()
	}
}

func (this *Query) isCloser(contact PacketContact) bool {
	this.RLock()
	defer this.RUnlock()

	return this.dht.routing.DistanceBetwin(contact.Hash, this.hash) < this.dht.routing.DistanceBetwin(this.closest, this.hash)
}

func (this *Query) addToQueue(node *Node) {
	this.workerQueue.Add(func() *Response {
		return this.job(node)
	})
}

func (this *Query) WaitResult() *Response {
	storeAnswers := []bool{}

	for res := range this.workerQueue.Results {
		if res.Err != nil {
			this.workerQueue.OnDone()

			continue
		}

		if len(res.Data) > 0 {
			return res
		}

		if len(res.Contacts) > 0 {
			for _, contact := range res.Contacts {
				this.processContact(&contact)
			}
		} else {
			storeAnswers = append(storeAnswers, res.Ok)
		}

		this.workerQueue.OnDone()
	}

	if len(storeAnswers) > 0 {
		return &Response{
			Ok: true,
		}
	}

	res := &Response{}

	for _, best := range this.best {
		res.Contacts = append(res.Contacts, best.Contact)
	}

	return res
}

func (this *Query) processContact(contact *PacketContact) {
	if this.isContactBlacklisted(contact) || this.isOwn(contact) {
		return
	}

	if _, err := this.dht.routing.GetNode(contact.Hash); err == nil {
		return
	}

	n := NewNode(this.dht, *contact)

	// res := n.Ping()

	this.Lock()
	this.blacklist = append(this.blacklist, n)
	this.Unlock()

	// if res.Err != nil {
	// 	return
	// }

	this.tryAddToBest(n)
}

func (this *Query) tryAddToBest(node *Node) {
	this.addToQueue(node)

	if this.isCloser(node.Contact) {

		this.updateClosest(node.Contact)

		this.Lock()

		this.best = append(this.best, node)
		this.sortByClosest(this.best)

		smalest := len(this.best)

		if smalest > BUCKET_SIZE {
			smalest = BUCKET_SIZE
		}

		this.best = this.best[:smalest]
		this.Unlock()
	}
}

func (this *Query) sortByClosest(bucket []*Node) {
	sort.Slice(bucket, func(i, j int) bool {
		return this.dht.routing.DistanceBetwin(bucket[i].Contact.Hash, this.hash) < this.dht.routing.DistanceBetwin(bucket[j].Contact.Hash, this.hash)
	})
}

func (this *Query) isOwn(contact *PacketContact) bool {
	return compare(this.dht.hash, contact.Hash) == 0
}

func (this *Query) isContactBlacklisted(contact *PacketContact) bool {
	this.RLock()
	defer this.RUnlock()

	return this.contains(this.blacklist, contact)
}

func (this *Query) contains(bucket []*Node, node *PacketContact) bool {
	for _, n := range bucket {
		if compare(node.Hash, n.Contact.Hash) == 0 {
			return true
		}
	}

	return false
}

func (this *Query) contactContains(bucket []PacketContact, node *PacketContact) bool {
	for _, n := range bucket {
		if compare(node.Hash, n.Hash) == 0 {
			return true
		}
	}

	return false
}
