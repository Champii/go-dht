package dht

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sync"
)

type Routing struct {
	sync.RWMutex
	buckets [][]*Node
	dht     *Dht
}

func NewRouting() *Routing {
	buckets := make([][]*Node, HASH_SIZE)

	return &Routing{
		buckets: buckets,
	}
}

func (this *Routing) Print() {
	this.RLock()
	defer this.RUnlock()

	for i, bucket := range this.buckets {
		for _, node := range bucket {
			fmt.Println(i, hex.EncodeToString(node.Contact.Hash))
		}
	}
}

func (this *Routing) Size() int {
	this.RLock()
	defer this.RUnlock()

	count := 0

	for _, bucket := range this.buckets {
		for range bucket {
			count++
		}
	}

	return count
}

func (this *Routing) countSameBit(hash []byte) int {
	if len(hash) == 0 {
		return 0
	}

	count := 0
	for i, v := range this.dht.hash {
		for j := 0; j < 8; j++ {
			tmpOwn := v & (0x1 << uint(j))
			tmp := hash[i] & (0x1 << uint(j))
			if tmpOwn == tmp {
				count++
			} else {
				return count
			}
		}
	}

	return count
}

func (this *Routing) nCopy(dest, src []byte, count int) []byte {
	i := 0
	for i < count {
		if count-i%8 == 0 {
			dest[i/8] = src[i/8]
			i += 8
			continue
		}

		destTmp := dest[i/8]

		for j := 0; j < 8 && i+j < count; j++ {
			destTmp |= src[i/8] & (0x1 << uint(j))
		}

		dest[i/8] = destTmp

		i += 8
	}

	return dest
}

func (this *Routing) DistanceBetwin(hash1, hash2 []byte) int {
	var res int

	for i, v := range hash1 {
		res += int((v ^ hash2[i])) * int(math.Pow(8, float64(i)))
	}

	return res
}

func (this *Routing) AddNode(node *Node) {
	if _, err := this.GetNode(node.Contact.Hash); err == nil {
		return
	}

	if c, err := this.GetByAddr(node.Contact.Addr); err == nil {
		this.RemoveNode(c)
	}

	bucketNb := this.countSameBit(node.Contact.Hash)

	this.Lock()
	if bucketNb == HASH_SIZE || len(this.buckets[bucketNb]) > BUCKET_SIZE {
		this.Unlock()
		return
	}

	if !this.dht.onAddNode(node) {
		this.Unlock()
		return
	}

	this.buckets[bucketNb] = append(this.buckets[bucketNb], node)
	this.Unlock()

	this.dht.logger.Info(node, "+ Add Routing. Size: ", this.Size())
}

func (this *Routing) RemoveNode(node *Node) {
	bucketNb := this.countSameBit(node.Contact.Hash)

	if bucketNb == HASH_SIZE {
		return
	}

	size := this.Size() - 1

	this.Lock()
	defer this.Unlock()

	for i, n := range this.buckets[bucketNb] {
		if compare(n.Contact.Hash, node.Contact.Hash) == 0 {
			if i == 0 {
				this.buckets[bucketNb] = this.buckets[bucketNb][1:]
			} else if i == len(this.buckets[bucketNb])-1 {
				this.buckets[bucketNb] = this.buckets[bucketNb][:i]
			} else {
				this.buckets[bucketNb] = append(this.buckets[bucketNb][:i], this.buckets[bucketNb][i+1:]...)
			}

			this.dht.logger.Info(n, "- Del Routing. Size: ", size)

			if node.Client != nil && !node.Client.IsClosing() && !node.Client.IsShutdown() {
				node.Client.Close()
			}

			if size == 0 && len(this.dht.options.BootstrapAddr) != 0 {
				this.dht.logger.Critical("Empty routing table. Stoping.")

				this.dht.Stop()
			}

			return
		}
	}

	// this.dht.logger.Warning(node, "x Cannot find node")
}

func (this *Routing) FindNode(hash []byte) []*Node {
	res := []*Node{}

	size := this.Size()

	this.RLock()
	defer this.RUnlock()

	if size < BUCKET_SIZE {
		return this.GetAllNodes()
	}

	bucketNb := this.countSameBit(hash)

	// get neighbours when asking for self
	if bucketNb == HASH_SIZE {
		bucketNb--
	}

	for len(res) < BUCKET_SIZE && bucketNb < HASH_SIZE && bucketNb >= 0 {
		for _, node := range this.buckets[bucketNb] {
			if len(res) == BUCKET_SIZE {
				return res
			}

			res = append(res, node)
		}

		bucketNb--
	}

	bucketNb = this.countSameBit(hash) + 1

	// if result bucket not full, add some more nodes
	for len(res) < BUCKET_SIZE && bucketNb >= 0 {
		for _, node := range this.buckets[bucketNb] {
			if len(res) == BUCKET_SIZE {
				return res
			}

			res = append(res, node)
		}

		bucketNb++
	}

	return res
}

func (this *Routing) FindNodeAndOwn(hash []byte) []*Node {
	res := []*Node{}

	size := this.Size()

	this.RLock()
	defer this.RUnlock()

	if size < BUCKET_SIZE {
		return this.GetAllNodes()
	}

	bucketNb := this.countSameBit(hash)

	for len(res) < BUCKET_SIZE && bucketNb <= HASH_SIZE && bucketNb >= 0 {
		for _, node := range this.buckets[bucketNb] {
			if len(res) == BUCKET_SIZE {
				return res
			}

			res = append(res, node)
		}

		bucketNb--
	}

	bucketNb = this.countSameBit(hash) + 1

	// if result bucket not full, add some more nodes
	for len(res) < BUCKET_SIZE && bucketNb >= 0 {
		for _, node := range this.buckets[bucketNb] {
			if len(res) == BUCKET_SIZE {
				return res
			}

			res = append(res, node)
		}

		bucketNb++
	}

	return res
}

// not tested
func (this *Routing) FindNearestNode(hash []byte) *Node {
	this.RLock()
	defer this.RUnlock()

	nodes := this.FindNode(hash)

	res := nodes[0]
	min := this.DistanceBetwin(nodes[0].Contact.Hash, hash)

	for _, node := range nodes {
		dist := this.DistanceBetwin(node.Contact.Hash, hash)

		if dist < min {
			min = dist
			res = node
		}
	}

	return res
}

func (this *Routing) GetNode(hash []byte) (*Node, error) {
	bucketNb := this.countSameBit(hash)

	if bucketNb == HASH_SIZE {
		return &Node{}, errors.New("Cannot get own")
	}

	this.RLock()
	defer this.RUnlock()

	for _, node := range this.buckets[bucketNb] {
		if compare(node.Contact.Hash, hash) == 0 {
			return node, nil
		}
	}

	return &Node{}, errors.New("Not found")
}

func (this *Routing) IsBestStorage(hash []byte) (bool, []*Node) {
	bucket := this.FindNode(hash)

	dist1 := this.countSameBit(hash)

	smalest := dist1

	for _, node := range bucket {
		dist2 := this.countSameBit(node.Contact.Hash)

		if dist2 < smalest {
			smalest = dist2
		}
	}

	if dist1 > smalest {
		return false, bucket
	}

	return true, []*Node{}
}

func (this *Routing) IsNearestOf(origin []byte, toCheck []byte) bool {
	bucket := this.FindNode(toCheck)

	dist1 := this.DistanceBetwin(origin, toCheck)

	smalest := dist1

	for _, node := range bucket {
		dist2 := this.DistanceBetwin(origin, node.Contact.Hash)

		if dist2 < smalest {
			return false
		}
	}

	return true
}

func (this *Routing) GetAllNodes() []*Node {
	res := []*Node{}

	this.RLock()
	defer this.RUnlock()

	for i := 0; i < BUCKET_SIZE; i++ {
		for _, node := range this.buckets[i] {
			res = append(res, node)
		}
	}

	return res
}

func (this *Routing) GetByAddr(addr string) (*Node, error) {
	this.RLock()
	defer this.RUnlock()

	for i := 0; i < BUCKET_SIZE; i++ {
		for _, node := range this.buckets[i] {
			if addr == node.Contact.Addr {
				return node, nil
			}
		}
	}

	return &Node{}, errors.New("Not found")
}
