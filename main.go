package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"pucrs/sd/raft"
	"strconv"
	"strings"
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
	ready := make(chan any)
	server := raft.NewServer(id, peerIds, ready, commitChan)
	server.Serve(listen)

	connectToPeers(server, peerAddrs)

	close(ready)

	// Print committed entries
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

func connectToPeers(server *raft.Server, peerIdsToAddrs map[int]string) {
	for id, addr := range peerIdsToAddrs {
		go func(id int, addr string) {
			err := retry.Do(
				func() error {
					tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
					if err != nil {
						return err
					}
					if err := server.ConnectToPeer(id, tcpAddr); err != nil {
						return err
					}
					log.Printf("connected to peer %d at %s", id, addr)
					return nil
				},
				retry.Attempts(5),
				retry.DelayType(retry.BackOffDelay),
				retry.Delay(1*time.Second),
			)
			if err != nil {
				log.Printf("exhausted all attempts to connect to peer %d at %s: %v", id, addr, err)
			}
		}(id, addr)
	}
}
