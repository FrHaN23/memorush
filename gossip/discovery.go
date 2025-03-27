package gossip

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"sync/atomic"
	"time"
)

type DiscoveryMessage struct {
	Type      string            `json:"type"`      // "discovery"
	SenderID  string            `json:"sender_id"` // Node ID of the sender
	SenderIP  string            `json:"sender_ip"` // Address of the sender
	Peers     map[string]string `json:"peers"`     // Known peers (ID -> Address)
	Signature string            `json:"signature"` // Optional: for verification
}

func (n *Node) handleDiscoveryMessage(data []byte, addr *net.UDPAddr) {
	var msg DiscoveryMessage
	err := json.Unmarshal(data, &msg)
	if err != nil {
		log.Println("Invalid discovery message:", err)
		return
	}

	// Verify that the message's SenderIP matches the actual sender's address
	if addr.String() != msg.SenderIP {
		log.Printf("Warning: SenderIP mismatch! Expected %s, got %s", addr.String(), msg.SenderIP)
		return
	}

	n.mu.Lock()
	if _, exists := n.Peers[msg.SenderID]; !exists {
		n.Peers[msg.SenderID] = &PeerInfo{ID: msg.SenderID, Address: msg.SenderIP}
		log.Printf("Discovered new peer: %s at %s", msg.SenderID, msg.SenderIP)
	}
	n.mu.Unlock()
}

func (n *Node) BroadcastPresence() {
	conn, err := net.Dial("udp", "255.255.255.255:9999") // Broadcast UDP
	if err != nil {
		log.Fatalf("Failed to create broadcast socket: %v", err)
	}
	defer conn.Close()

	for {
		msg := fmt.Sprintf("DISCOVERY:%s:%s", n.ID, n.Address)
		_, err := conn.Write([]byte(msg))
		if err != nil {
			log.Printf("Error broadcasting presence: %v", err)
		}

		time.Sleep(5 * time.Second) // Adjust interval as needed
	}
}

func (n *Node) ListenForPeers() {
	addr, err := net.ResolveUDPAddr("udp", n.Address)
	if err != nil {
		log.Fatalf("[%s] ERROR: Failed to resolve UDP address %s: %v", n.ID, n.Address, err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("[%s] ERROR: Failed to start UDP listener on %s: %v", n.ID, n.Address, err)
	}
	defer conn.Close()

	log.Printf("[%s] Listening for peers on %s", n.ID, n.Address)

	buf := make([]byte, 1024)

	for {
		nRead, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("[%s] ERROR: Failed to read UDP packet: %v", n.ID, err)
			continue
		}

		message := string(buf[:nRead])
		log.Printf("[%s] Received message from %s: %s", n.ID, remoteAddr.String(), message)

		// ✅ Skip processing if the message is from itself
		if remoteAddr.String() == n.Address {
			log.Printf("[%s] Ignoring message from self (%s)", n.ID, remoteAddr.String())
			continue
		}

		if strings.HasPrefix(message, "DISCOVERY") {
			parts := strings.Split(message, ":")
			if len(parts) == 3 {
				peerID := parts[1]
				peerAddress := parts[2]

				// ✅ Skip if the discovered peer is itself
				if peerID == n.ID || peerAddress == n.Address {
					log.Printf("[%s] Ignoring self-discovery message.", n.ID)
					continue
				}

				n.mu.Lock()
				if n.Peers == nil {
					n.Peers = make(map[string]*PeerInfo)
				}

				if _, exists := n.Peers[peerID]; !exists {
					n.Peers[peerID] = &PeerInfo{ID: peerID, Address: peerAddress}
					log.Printf("[%s] Discovered peer: %s (%s)", n.ID, peerID, peerAddress)
				}
				n.mu.Unlock()

				// Respond to sender with our address
				response := fmt.Sprintf("DISCOVERY_RESPONSE:%s:%s", n.ID, n.Address)
				_, err := conn.WriteToUDP([]byte(response), remoteAddr)
				if err != nil {
					log.Printf("[%s] ERROR: Failed to send discovery response: %v", n.ID, err)
				}
			}
		} else if strings.HasPrefix(message, "DISCOVERY_RESPONSE") {
			parts := strings.Split(message, ":")
			if len(parts) == 3 {
				peerID := parts[1]
				peerAddress := parts[2]

				// ✅ Skip if the response is from itself
				if peerID == n.ID || peerAddress == n.Address {
					log.Printf("[%s] Ignoring self-discovery response.", n.ID)
					continue
				}

				n.mu.Lock()
				if n.Peers == nil {
					n.Peers = make(map[string]*PeerInfo)
				}

				if _, exists := n.Peers[peerID]; !exists {
					n.Peers[peerID] = &PeerInfo{ID: peerID, Address: peerAddress}
					log.Printf("[%s] Received discovery response from: %s (%s)", n.ID, peerID, peerAddress)
				}
				n.mu.Unlock()
			}
		}
	}
}

func (n *Node) ProcessDiscoveryMessage(msg string, addr *net.UDPAddr) {
	parts := strings.Split(msg, ":")
	if len(parts) != 3 || parts[0] != "DISCOVERY" {
		log.Printf("Invalid discovery message: %s", msg)
		return
	}

	peerID := parts[1]
	peerAddress := parts[2]

	n.mu.Lock()
	defer n.mu.Unlock()

	// Ignore if it's our own ID
	if peerID == n.ID {
		return
	}

	// Add peer if not already known
	if _, exists := n.Peers[peerID]; !exists {
		n.Peers[peerID] = &PeerInfo{Address: peerAddress}
		log.Printf("Discovered new peer: %s (%s)", peerID, peerAddress)
	}
}

func (n *Node) BroadcastDiscovery() {
	broadcastAddr, err := net.ResolveUDPAddr("udp", "255.255.255.255:9999") // network wide
	if err != nil {
		log.Fatalf("Failed to resolve broadcast address: %v", err)
	}

	conn, err := net.DialUDP("udp", nil, broadcastAddr)
	if err != nil {
		log.Fatalf("Failed to broadcast discovery: %v", err)
	}
	defer conn.Close()

	for {
		if atomic.LoadUint32(&n.shutdown) == 1 {
			return
		}

		msg := DiscoveryMessage{
			SenderID: n.ID,
			SenderIP: n.Address,
		}
		data, _ := json.Marshal(msg)

		_, err := conn.Write(data)
		if err != nil {
			log.Println("Failed to send discovery broadcast:", err)
		}

		log.Println("Sent discovery broadcast")
		time.Sleep(5 * time.Second) // Adjust as needed
	}
}

func (n *Node) GossipPeers() {
	ticker := time.NewTicker(10 * time.Second) // Gossip every 10s
	defer ticker.Stop()

	for range ticker.C {
		n.mu.Lock()
		peerList := strings.Join(n.getPeerAddresses(), ",")
		n.mu.Unlock()

		for peerAddr := range n.Peers {
			n.sendPeerList(peerAddr, peerList)
		}
	}
}

func (n *Node) sendPeerList(peerAddr, peerList string) {
	conn, err := net.Dial("udp", peerAddr)
	if err != nil {
		log.Printf("Failed to connect to peer %s: %v", peerAddr, err)
		return
	}
	defer conn.Close()

	msg := fmt.Sprintf("PEERS %s", peerList)
	_, err = conn.Write([]byte(msg))
	if err != nil {
		log.Printf("Failed to send peer list to %s: %v", peerAddr, err)
	}
}

func (n *Node) getPeerAddresses() []string {
	addresses := []string{}
	for _, peer := range n.Peers {
		addresses = append(addresses, peer.Address)
	}
	return addresses
}
