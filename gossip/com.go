package gossip

import (
	"encoding/json"
	"log"
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

		// Handle the gossip message in a separate goroutine
		go n.handleGossip(data, remoteAddr)
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

		// Pick a random peer
		var peerList []*PeerInfo
		for _, peer := range n.Peers {
			peerList = append(peerList, peer)
		}
		rand.Shuffle(len(peerList), func(i, j int) { peerList[i], peerList[j] = peerList[j], peerList[i] })
		targetPeer := peerList[0] // Pick the first after shuffle
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
		log.Printf("Sending gossip: %s\n", string(data))
		n.sendToPeer(targetPeer.Address, data)
	}
}
