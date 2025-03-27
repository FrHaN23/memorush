package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/frhan23/memorush/gossip"
)

// GetAvailablePort finds a free port dynamically
func GetAvailablePort() int {
	addr, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	l, _ := net.ListenTCP("tcp", addr)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// Starts a single node process
func startNode(port int) {
	address := fmt.Sprintf("127.0.0.1:%d", port)
	node := gossip.NewNode(fmt.Sprintf("node-%d", port), address)

	// Wait a bit before gossiping
	time.Sleep(2 * time.Second)
	go node.SendGossip()

	// Keep running
	select {}
}

// Main function to start multiple independent processes
func main() {
	if len(os.Args) > 1 {
		// Start a node process if called with a port argument
		port, _ := strconv.Atoi(os.Args[1])
		startNode(port)
		return
	}

	// Otherwise, start a distributed test setup
	nodeCounts := []int{3} // Different test scenarios

	for _, count := range nodeCounts {
		fmt.Printf("Launching %d nodes...\n", count)

		processes := []*exec.Cmd{}
		peerAddresses := []string{} // To store peer addresses

		// Start nodes
		for i := 0; i < count; i++ {
			port := GetAvailablePort()

			// Keep track of the first few nodes as initial peers
			if i < 3 {
				peerAddresses = append(peerAddresses, fmt.Sprintf("127.0.0.1:%d", port))
			}

			// Create log file for each node
			logFileName := fmt.Sprintf("node_%d.log", port)
			logFile, err := os.Create(logFileName)
			if err != nil {
				log.Fatalf("Failed to create log file for node %d: %v", port, err)
			}
			defer logFile.Close()

			// Start a separate node process with known peers
			args := append([]string{strconv.Itoa(port)}, peerAddresses...)
			cmd := exec.Command(os.Args[0], args...) // Run self with port & peer list
			cmd.Stdout = logFile
			cmd.Stderr = logFile

			err = cmd.Start()
			if err != nil {
				log.Fatalf("Failed to start node on port %d: %v", port, err)
			}

			processes = append(processes, cmd)
			fmt.Printf("Node-%d started on port %d (PID: %d), logs: %s\n", i, port, cmd.Process.Pid, logFileName)
		}

		// Handle graceful shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		go func() {
			<-sigChan
			fmt.Println("\nReceived termination signal. Stopping nodes...")
			for _, process := range processes {
				if err := process.Process.Kill(); err != nil {
					log.Printf("Failed to kill process (PID: %d): %v", process.Process.Pid, err)
				} else {
					fmt.Printf("Stopped node (PID: %d)\n", process.Process.Pid)
				}
			}
			os.Exit(0)
		}()

		// Run for 30 seconds before stopping
		time.Sleep(30 * time.Second)

		// Stop nodes
		fmt.Println("Stopping nodes after test duration...")
		for _, process := range processes {
			if err := process.Process.Kill(); err != nil {
				log.Printf("Failed to stop process (PID: %d): %v", process.Process.Pid, err)
			} else {
				fmt.Printf("Stopped node (PID: %d)\n", process.Process.Pid)
			}
		}
	}
}
