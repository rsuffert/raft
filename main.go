package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"pucrs/sd/raft"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/avast/retry-go"
)

const logStateInterval time.Duration = 5 * time.Second

func main() {
	var id int
	var peers string
	var listen string
	flag.IntVar(&id, "id", 0, "server ID")
	flag.StringVar(&peers, "peers", "", "comma-separated list of peerID=ip:port")
	flag.StringVar(&listen, "listen", ":9000", "address to listen on")
	flag.Parse()

	peerIds, peerAddrs, err := parsePeersStr(peers)
	if err != nil {
		log.Fatalf("failed to parse peers: %v", err)
	}

	commitChan := make(chan raft.CommitEntry)
	defer close(commitChan)
	ready := make(chan any)

	server := raft.NewServer(id, peerIds, ready, commitChan)
	server.Serve(listen)

	// asynchronously connect to peers
	var wg sync.WaitGroup
	for peerId, peerAddr := range peerAddrs {
		wg.Add(1)
		go func(peerId int, peerAddr string) {
			defer wg.Done()
			if err := connectToPeer(server, peerId, peerAddr); err != nil {
				log.Printf("failed to connect to peer %d at %s: %v", peerId, peerAddr, err)
			} else {
				log.Printf("successfully connected to peer %d at %s", peerId, peerAddr)
			}
		}(peerId, peerAddr)
	}
	wg.Wait()
	close(ready)

	go randomlySubmitCommands(server, id)

	ticker := time.NewTicker(logStateInterval)
	for {
		select {
		case <-ticker.C:
			// log the state of the server periodically
			id, term, isLeader, logEntries := server.Report()
			log.Printf("server %d, term %d, isLeader: %t, log entries: %d", id, term, isLeader, len(logEntries))
		case entry := <-commitChan:
			// the consensus module has committed an entry
			log.Printf("committed entry: %+v", entry)
		}
	}
}

// parsePeersStr parses a comma-separated string of peer definitions into a slice of peer IDs and a map from peer IDs to their addresses.
// The input string should be in the format "peerID1=ip1:port1,peerID2=ip2:port2,..."
// Returns a slice of peer IDs, a map from peer IDs to their corresponding addresses, and an error if the input format is invalid.
func parsePeersStr(peers string) (peerIds []int, peerIdsToAddrs map[int]string, err error) {
	peerIds = make([]int, 0)
	peerIdsToAddrs = make(map[int]string)

	for _, p := range strings.Split(peers, ",") {
		if len(p) == 0 {
			continue
		}

		parts := strings.SplitN(p, "=", 2)
		if len(parts) != 2 {
			return nil, nil, fmt.Errorf("invalid peer format: %s, expected peerID=ip:port", p)
		}

		id, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, nil, fmt.Errorf("invalid peer ID %s: %v", parts[0], err)
		}

		peerIds = append(peerIds, id)
		peerIdsToAddrs[id] = parts[1]
	}

	return peerIds, peerIdsToAddrs, nil
}

// connectToPeer attempts to establish a connection to a peer Raft server with the specified peerId and peerAddr.
// It retries the connection up to 5 times with exponential backoff, waiting 1 second between attempts.
// Returns an error if the connection could not be established after all retries.
func connectToPeer(server *raft.Server, peerId int, peerAddr string) error {
	return retry.Do(
		func() error {
			tcpAddr, err := net.ResolveTCPAddr("tcp", peerAddr)
			if err != nil {
				return err
			}
			if err := server.ConnectToPeer(peerId, tcpAddr); err != nil {
				return err
			}
			return nil
		},
		retry.Attempts(5),
		retry.DelayType(retry.BackOffDelay),
		retry.Delay(1*time.Second),
	)
}

// randomlySubmitCommands periodically generates and submits random commands to the given Raft server.
// Each command is uniquely identified by the peerId and the current timestamp in milliseconds.
// The function runs indefinitely, waiting a random interval between 3 and 7 seconds before each submission.
// If the command is successfully submitted, a success message is logged; otherwise, a failure message is logged.
//
// Parameters:
//
//	server - Pointer to the raft.Server instance to which commands will be submitted.
//	peerId - Integer identifier for the peer generating the commands.
func randomlySubmitCommands(server *raft.Server, peerId int) {
	for {
		time.Sleep(time.Duration(3+rand.Intn(5)) * time.Second) // Random delay between 3 and 7 seconds
		cmd := fmt.Sprintf("auto-cmd-%d-%d", peerId, time.Now().UnixMilli())
		if ok := server.Submit(cmd); ok {
			log.Printf("submitted command: %s", cmd)
		}
	}
}
