package gossip

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"time"

	"github.com/frhan23/memorush/cache"
	"golang.org/x/time/rate"
)

type GossipMessage struct {
	Type      string                `json:"type"`
	SenderID  string                `json:"sender_id"`
	SenderIP  string                `json:"sender_ip"`
	Peers     map[string]string     `json:"peers"`
	TargetID  string                `json:"target_id,omitempty"`
	Signature string                `json:"signature"`
	CacheData map[string]cache.CacheItem `json:"cache_data,omitempty"`
}

var secretKey = []byte("your-secure-key")
var limiter = rate.NewLimiter(1, 5)

func signMessage(msg GossipMessage) string {
	data, _ := json.Marshal(msg)
	h := hmac.New(sha256.New, secretKey)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func verifySignature(msg GossipMessage) bool {
	expectedSig := signMessage(msg)
	return hmac.Equal([]byte(expectedSig), []byte(msg.Signature))
}

func (n *Node) startPing() {
	ticker := time.NewTicker(2 * time.Second) // Adjust interval as needed
	defer ticker.Stop()

	for range ticker.C {
		n.mu.Lock()
		for id, peer := range n.Peers {
			if time.Since(peer.LastAck) > 5*time.Second { // Timeout threshold
				peer.Status = Suspect
				log.Printf("Peer %s is suspected dead\n", id)
				go n.indirectProbe(id) // Try indirect probing
			} else {
				n.sendPing(peer.Address)
			}
		}
		n.mu.Unlock()
	}
}

func (n *Node) sendPing(peerAddr string) {
	msg := GossipMessage{
		Type:     "PING",
		SenderID: n.ID,
	}
	msg.Signature = signMessage(msg)

	data, err := json.Marshal(msg)
	if err != nil {
		log.Println("Failed to encode PING:", err)
		return
	}

	n.sendToPeer(peerAddr, data)
}

func (n *Node) sendToPeer(peerAddr string, data []byte) {
	if n.rateLimit.Allow(peerAddr) {
		log.Printf("Rate limit exceeded for peer %s, skipping message\n", peerAddr)
		return
	}

	conn, err := net.Dial("udp", peerAddr)
	if err != nil {
		log.Println("Failed to send to peer:", err)
		return
	}
	defer conn.Close()

	_, err = conn.Write(data)
	if err != nil {
		log.Println("Failed to send data:", err)
	}
}

func (n *Node) sendIndirectPing(suspectID, intermediaryAddr string) {
	msg := GossipMessage{
		Type:     "INDIRECT_PING",
		SenderID: n.ID,
		TargetID: suspectID,
	}
	msg.Signature = signMessage(msg)

	data, err := json.Marshal(msg)
	if err != nil {
		log.Println("Failed to encode INDIRECT_PING:", err)
		return
	}

	n.sendToPeer(intermediaryAddr, data)
}

func (n *Node) serializePeers() map[string]string {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Ensure map is not nil
	if n.Peers == nil {
		return make(map[string]string) // Return empty map instead of nil
	}

	peers := make(map[string]string, len(n.Peers)) // Pre-allocate space
	for id, peer := range n.Peers {
		peers[id] = peer.Address
	}
	return peers
}

func (n *Node) indirectProbe(suspectID string) {
	successCount := 0

	for id, peer := range n.Peers {
		if id == suspectID || peer.Status != Alive {
			continue
		}

		n.sendIndirectPing(suspectID, peer.Address)
		time.Sleep(500 * time.Millisecond) // Prevent flooding

		// If a PONG is received, increase success count
		n.mu.Lock()
		if n.Peers[suspectID].Status == Alive {
			successCount++
		}
		n.mu.Unlock()

		if successCount > 0 {
			break // Stop probing if any indirect ping succeeds
		}
	}

	if successCount == 0 {
		n.mu.Lock()
		n.Peers[suspectID].Status = Dead
		n.mu.Unlock()
		log.Printf("Peer %s is now marked as Dead\n", suspectID)
	}
}

func (n *Node) ProcessCacheUpdate(data []byte, addr *net.UDPAddr) {
	cacheSnapshot, err := deserializeCacheSnapshot(data)
	if err != nil {
		log.Printf("Failed to deserialize cache update: %v", err)
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	for key, item := range cacheSnapshot {
		localItem, found := n.Cache.InspectItem(key)
		
		// Update if:
		// - Key doesn't exist locally OR
		// - The received item has a more recent LastAccessed time
		if !found || item.LastAccessed.After(localItem.LastAccessed) {
			n.Cache.Set(key, item.Value, item.TTL)
			log.Printf("Updated cache from peer: %s -> %v", key, item.Value)
		}
	}
}
