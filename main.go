package main

import (
	"fmt"
	"log"
	"os"
	"runtime/pprof"
	"time"

	"github.com/frhan23/memorush/gossip"
)

func main() {
	
	f, err := os.Create("cpu.prof")
    if err != nil {
        panic(err)
    }
    defer f.Close()

    // Start profiling
    pprof.StartCPUProfile(f)
    defer pprof.StopCPUProfile()
	
	nodeCount := []int{3, 10, 50, 100} // Different node counts for testing
	for _, count := range nodeCount {
		fmt.Printf("Testing with %d nodes...\n", count)
		nodes := make([]*gossip.Node, count)

		// Start nodes
		for i := 0; i < count; i++ {
			address := fmt.Sprintf("127.0.0.1:%d", 8000+i)
			node := gossip.NewNode(fmt.Sprintf("node-%d", i), address)
			nodes[i] = node
			go node.StartListening()
		}

		// Allow nodes to initialize
		time.Sleep(2 * time.Second)

		// Start gossiping
		for _, node := range nodes {
			go node.SendGossip()
		}

		// Run for 30 seconds and collect stats
		time.Sleep(30 * time.Second)

		// Evaluate performance (manually check logs or implement monitoring)
		log.Printf("Completed test with %d nodes", count)

		// Stop nodes
		for _, node := range nodes {
			node.Shutdown() // Implement graceful shutdown if needed
		}
	}
}
