package dht

import (
	"encoding/hex"
	"fmt"
	"math"
	"os"
)

type Routing struct {
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
	for i, bucket := range this.buckets {
		for _, node := range bucket {
			if node != nil {
				fmt.Println(i, node.contact.Hash)
			}
		}
	}
}

func (this *Routing) Size() int {
	count := 0

	for _, bucket := range this.buckets {
		for range bucket {
			count++
		}
	}

	return count
}

func (this *Routing) countSameBit(hash_ string) int {
	ownHash, _ := hex.DecodeString(this.dht.hash)
	hash, _ := hex.DecodeString(hash_)

	count := 0
	for i, v := range ownHash {
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

func (this *Routing) distanceBetwin(hash1, hash2 string) int {
	ownHash, _ := hex.DecodeString(hash1)
	hash, _ := hex.DecodeString(hash2)

	var res int

	for i, v := range ownHash {
		res += int((v ^ hash[i])) * int(math.Pow(10, float64(i)))
	}

	return res
}

func (this *Routing) AddNode(node *Node) {
	if this.GetNode(node.contact.Hash) != nil {
		this.dht.logger.Debug(node, "x Routing already has this node")

		return
	}

	bucketNb := this.countSameBit(node.contact.Hash)

	if bucketNb >= BUCKET_SIZE {
		this.dht.logger.Debug(node, "x Bucket full", bucketNb)

		return
	}

	this.buckets[bucketNb] = append(this.buckets[bucketNb], node)
	this.dht.logger.Debug(node, "+ Add Routing. Size: ", this.Size())
}

func (this *Routing) RemoveNode(node *Node) {
	bucketNb := this.countSameBit(node.contact.Hash)

	for i, n := range this.buckets[bucketNb] {
		if n.contact.Hash == node.contact.Hash {
			this.buckets[bucketNb] = append(this.buckets[bucketNb][:i], this.buckets[bucketNb][i+1:]...)

			this.dht.logger.Debug(node, "- Del Routing. Size: ", this.Size())

			if this.Size() == 0 && len(this.dht.options.BootstrapAddr) != 0 {
				this.dht.logger.Critical("Empty routing table. Exiting.")
				os.Exit(1)
			}

			return
		}
	}

	this.dht.logger.Error(node, "x Cannot find node")
}

func (this *Routing) FindNode(hash string) []*Node {
	res := []*Node{}

	bucketNb := this.countSameBit(hash)

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

	return res
}

func (this *Routing) GetNode(hash string) *Node {
	bucketNb := this.countSameBit(hash)

	if bucketNb == HASH_SIZE {
		return nil
	}

	for _, node := range this.buckets[bucketNb] {
		if node.contact.Hash == hash {
			return node
		}
	}

	return nil
}
