package dht

import "sort"
import "fmt"

type QueryJob func(*Node) chan interface{}

type Query struct {
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
		job:         job,
		dht:         dht,
		hash:        hash,
		closest:     dht.hash,
		workerQueue: NewWorkerQueue(3, 100),
		result:      make(chan interface{}, 1),
	}
}

func (this *Query) Run() interface{} {
	bucket := this.dht.routing.FindNode(this.hash)

	if len(bucket) == 0 {
		return []*Node{}
	}

	this.best = bucket
	this.sortByClosest(this.best)

	this.blacklist = bucket

	for _, node := range bucket {
		this.updateClosest(node)
		this.addToQueue(node)
	}

	this.workerQueue.Start()

	return this.WaitResult()
}

func (this *Query) updateClosest(node *Node) {
	if this.isCloser(node) {
		this.closest = node.contact.Hash
	}
}

func (this *Query) isCloser(node *Node) bool {
	return this.dht.routing.distanceBetwin(node.contact.Hash, this.hash) < this.dht.routing.distanceBetwin(this.closest, this.hash)
}

func (this *Query) addToQueue(node *Node) {
	this.workerQueue.Add(func() chan interface{} {
		return this.job(node)
	})
}

func (this *Query) WaitResult() interface{} {
	storeAnswers := []bool{}

	for res := range this.workerQueue.Results {
		switch res.(type) {
		case error:
			fmt.Println(res)
			continue
		case Packet:
			if res.(Packet).Header.Command == COMMAND_FOUND {
				return res.(Packet).Data
			}

			switch res.(Packet).Data.(type) {
			case bool:
				storeAnswers = append(storeAnswers, res.(Packet).Data.(bool))
			case []PacketContact:
				toAdd := res.(Packet).Data.([]PacketContact)

				for _, contact := range toAdd {
					this.processContact(contact)
				}
			}
		default:
		}
	}

	if len(storeAnswers) > 0 {
		return storeAnswers
	}

	return this.best
}

func (this *Query) processContact(contact PacketContact) {
	if this.isContactBlacklisted(contact) || this.isOwn(contact) {
		return
	}

	if this.dht.routing.GetNode(contact.Hash) != nil {
		return
	}

	n := NewNode(this.dht, contact.Addr, contact.Hash)

	if err := n.Connect(); err != nil {
		return
	}

	this.blacklist = append(this.blacklist, n)

	this.tryAddToBest(n)

	if this.isCloser(n) {
		this.addToQueue(n)
	}
}

func (this *Query) tryAddToBest(node *Node) {
	if this.isCloser(node) {
		this.updateClosest(node)

		this.best = append(this.best, node)
		this.sortByClosest(this.best)

		smalest := len(this.best)

		if smalest > BUCKET_SIZE {
			smalest = BUCKET_SIZE
		}

		this.best = this.best[:smalest]
	}
}

func (this *Query) sortByClosest(bucket []*Node) {
	sort.Slice(bucket, func(i, j int) bool {
		return this.dht.routing.distanceBetwin(bucket[i].contact.Hash, this.hash) < this.dht.routing.distanceBetwin(bucket[j].contact.Hash, this.hash)
	})
}

func (this *Query) isOwn(contact PacketContact) bool {
	return compare(this.dht.hash, contact.Hash) == 0
}

func (this *Query) isContactBlacklisted(contact PacketContact) bool {
	return this.contains(this.blacklist, contact)
}

func (this *Query) contains(bucket []*Node, node PacketContact) bool {
	for _, n := range bucket {
		if compare(node.Hash, n.contact.Hash) == 0 {
			return true
		}
	}

	return false
}

func (this *Query) contactContains(bucket []PacketContact, node PacketContact) bool {
	for _, n := range bucket {
		if compare(node.Hash, n.Hash) == 0 {
			return true
		}
	}

	return false
}
