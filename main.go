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

	// begin submitting commands for consensus randomly and print those that are committed
	go randomlySubmitCommands(server)
	for entry := range commitChan {
		fmt.Printf("Committed: %+v\n", entry)
	}
}

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
			log.Printf("connected to peer %d at %s", peerId, peerAddr)
			return nil
		},
		retry.Attempts(5),
		retry.DelayType(retry.BackOffDelay),
		retry.Delay(1*time.Second),
	)
}

func randomlySubmitCommands(server *raft.Server) {
	for {
		time.Sleep(time.Duration(3+rand.Intn(5)) * time.Second) // Random delay between 3 and 7 seconds
		cmd := fmt.Sprintf("auto-cmd-%d", rand.Intn(10000))
		if ok := server.Submit(cmd); ok {
			log.Printf("submitted command: %s", cmd)
		} else {
			log.Printf("failed to submit command: %s", cmd)
		}
	}
}
