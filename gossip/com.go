package gossip

import (
	"encoding/json"
	"log"
	"math"
	"math/rand"
	"net"
	"sync/atomic"
	"time"
)

func (n *Node) StartListening() {
	addr, err := net.ResolveUDPAddr("udp", n.Address)
	if err != nil {
		log.Fatalf("Failed to resolve address: %v", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	n.listener = conn
	defer conn.Close()

	buf := make([]byte, 1024) // 1 KB buffer

	for {
		if atomic.LoadUint32(&n.shutdown) == 1 {
			log.Printf("%s: Listener shutting down...", n.ID)
			return
		}

		conn.SetReadDeadline(time.Now().Add(1 * time.Second)) // Prevents indefinite blocking

		nRead, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("Read error: %v", err)
			continue
		}

		// Copy the received data to prevent buffer overwrites
		data := make([]byte, nRead)
		copy(data, buf[:nRead])

		// Log received raw message
		log.Printf("Received from %s: %s", remoteAddr.String(), string(data))

		// Handle discovery messages
		go n.handleDiscoveryMessage(data, remoteAddr)
	}
}


func (n *Node) SendGossip() {
	for {
		if atomic.LoadUint32(&n.shutdown) == 1 {
			return
		}
		time.Sleep(time.Duration(rand.Intn(3)+1) * time.Second) // Random interval

		n.mu.Lock()
		peerCount := len(n.Peers)
		if peerCount == 0 {
			n.mu.Unlock()
			continue
		}

		targetCount := max(int(math.Sqrt(float64(peerCount))), 1)

		// Pick a random peer
		var peerList []*PeerInfo
		for _, peer := range n.Peers {
			peerList = append(peerList, peer)
		}
		rand.Shuffle(len(peerList), func(i, j int) { peerList[i], peerList[j] = peerList[j], peerList[i] })
		selectedPeers := peerList[:targetCount]
		n.mu.Unlock()

		// Send gossip message
		msg := GossipMessage{
			SenderID: n.ID,
			SenderIP: n.Address,
			Peers:    n.serializePeers(),
		}
		data, err := json.Marshal(msg)
		if err != nil {
			log.Println("Failed to encode PING:", err)
			return
		}
		log.Printf("Sending gossip to %d peers\n", targetCount)
		for _, peer := range selectedPeers {
			go n.sendToPeer(peer.Address, data) // Send in a goroutine for concurrency
		}
	}
}

func (n *Node) ListenCacheUpdates() {
	addr, err := net.ResolveUDPAddr("udp", n.Address)
	if err != nil {
		log.Fatalf("Failed to resolve address %s: %v", n.Address, err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", n.Address, err)
	}
	defer conn.Close()

	log.Printf("Listening for cache updates on %s", n.Address)

	buf := make([]byte, 4096) // Buffer for incoming data
	for {
		nRead, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("Error reading UDP message: %v", err)
			continue
		}

		go n.ProcessCacheUpdate(buf[:nRead], remoteAddr)
	}
}
