package gossip

import (
	"encoding/json"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
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
	ID       string
	Address  string
	Peers    map[string]*PeerInfo // Known peers
	mu       sync.RWMutex
	listener *net.UDPConn
	shutdown uint32
}

// PeerInfo stores details about a known peer.
type PeerInfo struct {
	ID      string
	Address string
	Status  NodeStatus
	LastAck time.Time
}

// NewNode initializes a new gossip node.
func NewNode(id, address string) *Node {
	return &Node{
		ID:      id,
		Address: address,
		Peers:   make(map[string]*PeerInfo),
	}
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

	// Process additional peers in the message
	for id, addr := range msg.Peers {
		if id == n.ID || id == senderID {
			continue // Ignore self and sender
		}
		if _, found := n.Peers[id]; !found {
			n.Peers[id] = &PeerInfo{Address: addr, Status: Alive, LastAck: time.Now()}
			log.Printf("Learned about peer: %s -> %s\n", id, addr)
		}
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
