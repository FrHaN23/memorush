package gossip

import (
	"encoding/json"
	"log"
	"math/rand"
	"net"
	"time"
)

func (n *Node) StartListening() {
	addr, _ := net.ResolveUDPAddr("udp", n.Address)
	conn, _ := net.ListenUDP("udp", addr)
	n.listener = conn
	defer conn.Close()

	buf := make([]byte, 1024)
	for {
		nRead, remoteAddr, _ := conn.ReadFromUDP(buf)
		go n.handleGossip(buf[:nRead], remoteAddr)
	}
}

func (n *Node) SendGossip() {
	for {
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
		n.sendToPeer(targetPeer.Address, data)
	}
}
