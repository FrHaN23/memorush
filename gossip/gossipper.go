package gossip

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/frhan23/memorush/cache"
	"golang.org/x/time/rate"
)

// NodeStatus represents the state of a node.
type NodeStatus int

const (
	Alive NodeStatus = iota
	Suspect
	Dead
)

// Node represents a single cache node in the gossip network.
type Node struct {
	ID        string
	Address   string
	Peers     map[string]*PeerInfo // Known peers
	mu        sync.RWMutex
	listener  *net.UDPConn
	shutdown  uint32
	rateLimit *RateLimiter
	Cache     *cache.Cache
}

// PeerInfo stores details about a known peer.
type PeerInfo struct {
	ID      string
	Address string
	Status  NodeStatus
	LastAck time.Time
}

type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
	}
}

// NewNode initializes a new gossip node.
func NewNode(id, address string) *Node {
	node := &Node{
		ID:        id,
		Address:   address,
		Peers:     make(map[string]*PeerInfo),
		rateLimit: NewRateLimiter(),
		Cache:     cache.NewCache(&cache.CacheConfig{}),
	}

	// Start peer discovery
	go node.ListenForPeers()
	go node.BroadcastPresence()

	go node.ListenCacheUpdates()

	go node.cleanupDeadPeers()

	return node
}

func (n *Node) handleGossip(data []byte, remoteAddr *net.UDPAddr) {
	var msg GossipMessage
	err := json.Unmarshal(data, &msg)
	if err != nil {
		log.Println("Failed to decode gossip:", err)
		return
	}

	if !verifySignature(msg) {
		log.Println("Received message with invalid signature! Possible attack.")
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	senderID := msg.SenderID
	senderAddr := remoteAddr.String() // Use the actual sender's address

	// Add or update sender in peer list
	peer, exists := n.Peers[senderID]
	if !exists {
		n.mu.Lock()
		n.Peers[senderID] = &PeerInfo{Address: senderAddr, Status: Alive, LastAck: time.Now()}
		n.mu.Unlock()
	} else {
		peer.LastAck = time.Now() // Update last known time
	}

	// Process cache updates
	for key, item := range msg.CacheData {
		n.mu.Lock()
		localItem, found := n.Cache.InspectItem(key)

		if !found || item.LastAccessed.After(localItem.LastAccessed) {
			n.Cache.Set(key, item)
			log.Printf("Updated cache: %s -> %v", key, item.Value)
		}
		n.mu.Unlock()
	}
}

func (n *Node) AddPeer(id, address string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, exists := n.Peers[id]; !exists {
		n.Peers[id] = &PeerInfo{ID: id, Address: address}
		log.Printf("%s: Added peer %s (%s)", n.ID, id, address)
	}
}

// ShutdownFlag to gracefully stop the node
func (n *Node) Shutdown() {
	atomic.StoreUint32(&n.shutdown, 1) // Signal shutdown
	if n.listener != nil {
		n.listener.Close() // Close the UDP socket
	}
	log.Printf("%s: Node shutdown complete.", n.ID)
}

func (n *Node) cleanupDeadPeers() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		n.mu.Lock()
		for id, peer := range n.Peers {
			if peer.Status == Dead && time.Since(peer.LastAck) > 2*time.Minute {
				delete(n.Peers, id)
				log.Printf("Removed dead peer: %s", id)
			}
		}
		n.mu.Unlock()
	}
}

func (n *Node) SyncCacheWithPeers() {
	for {
		time.Sleep(time.Second * 10) // Adjust interval as needed

		n.mu.Lock()
		cacheSnapshot := make(map[string]*cache.CacheItem)

		for key, elem := range n.Cache.GetItems() {
			item := elem.Value.(*cache.CacheItem)
			cacheSnapshot[key] = item
		}
		n.mu.Unlock()

		for _, peer := range n.Peers {
			go n.SendCacheUpdate(peer, cacheSnapshot)
		}
	}
}

func (n *Node) SendCacheUpdate(peer *PeerInfo, cacheSnapshot map[string]*cache.CacheItem) {
	// Serialize cacheSnapshot to send over UDP
	data, err := serializeCacheSnapshot(cacheSnapshot)
	if err != nil {
		log.Printf("Failed to serialize cache snapshot: %v", err)
		return
	}

	addr, err := net.ResolveUDPAddr("udp", peer.Address)
	if err != nil {
		log.Printf("Failed to resolve peer address %s: %v", peer.Address, err)
		return
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Printf("Failed to send cache update to %s: %v", peer.Address, err)
		return
	}
	defer conn.Close()

	_, err = conn.Write(data)
	if err != nil {
		log.Printf("Failed to write to peer %s: %v", peer.Address, err)
	}
}

func (rl *RateLimiter) Allow(peer string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[peer]
	if !exists {
		limiter = rate.NewLimiter(rate.Every(5*time.Second), 1) // Allow 1 request per 5 seconds
		rl.limiters[peer] = limiter
	}

	return !limiter.Allow()
}

func serializeCacheSnapshot(snapshot map[string]*cache.CacheItem) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(snapshot)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func deserializeCacheSnapshot(data []byte) (map[string]*cache.CacheItem, error) {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	var snapshot map[string]*cache.CacheItem
	err := dec.Decode(&snapshot)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}
