package dht

import "sort"

import "net"
import "sync"

type QueryJob func(*Node) chan interface{}

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

func (this *Query) Run() interface{} {
	bucket := this.dht.routing.FindNode(this.hash)

	if len(bucket) == 0 {
		return []*Node{}
	}

	this.workerQueue.Start()

	for _, contact := range bucket {
		addr, _ := net.ResolveUDPAddr("udp", contact.Addr)

		n := NewNodeContact(this.dht, addr, contact)
		this.Lock()
		this.best = append(this.best, n)
		this.blacklist = append(this.blacklist, n)
		this.Unlock()

		this.updateClosest(contact)
		this.addToQueue(n)
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

	return this.dht.routing.distanceBetwin(contact.Hash, this.hash) < this.dht.routing.distanceBetwin(this.closest, this.hash)
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
			this.workerQueue.OnDone()
			continue

		case Packet:
			packet := res.(Packet)

			switch packet.Header.Command {
			case Command_FOUND:
				return packet.GetFound()

			case Command_STORED:
				storeAnswers = append(storeAnswers, packet.GetOk())

			case Command_FOUND_NODES:
				toAdd := packet.GetFoundNodes().Nodes

				for _, contact := range toAdd {
					this.processContact(contact)
				}

			default:
			}
		default:
		}

		this.workerQueue.OnDone()
	}

	if len(storeAnswers) > 0 {
		return storeAnswers
	}

	return this.best
}

func (this *Query) processContact(contact *PacketContact) {
	if this.isContactBlacklisted(contact) || this.isOwn(contact) {
		return
	}

	if _, err := this.dht.routing.GetNode(contact.Hash); err == nil {
		return
	}

	addr, err := net.ResolveUDPAddr("udp", contact.Addr)

	if err != nil {
		return
	}

	n := NewNode(this.dht, addr, contact.Hash)

	this.Lock()
	this.blacklist = append(this.blacklist, n)
	this.Unlock()

	// _, ok := (<-n.Ping()).(error)

	// if ok {
	// 	return
	// }

	this.tryAddToBest(n)

	// if this.isCloser(n.contact) {
	// }
}

func (this *Query) tryAddToBest(node *Node) {
	this.addToQueue(node)

	if this.isCloser(node.contact) {

		this.updateClosest(node.contact)

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
		return this.dht.routing.distanceBetwin(bucket[i].contact.Hash, this.hash) < this.dht.routing.distanceBetwin(bucket[j].contact.Hash, this.hash)
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
		if compare(node.Hash, n.contact.Hash) == 0 {
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
