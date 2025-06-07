// Server container for a Raft Consensus Module. Exposes Raft to the network
// and enables RPCs between Raft peers.
//
// Eli Bendersky [https://eli.thegreenplace.net]
// This code is in the public domain.
package raft

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/rpc"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/avast/retry-go"
)

const maxReconnectRetries = 1000 // retry "indefinitely"

// Server wraps a raft.ConsensusModule along with a rpc.Server that exposes its
// methods as RPC endpoints. It also manages the peers of the Raft server. The
// main goal of this type is to simplify the code of raft.Server for
// presentation purposes. raft.ConsensusModule has a *Server to do its peer
// communication and doesn't have to worry about the specifics of running an
// RPC server.
type Server struct {
	mu sync.Mutex

	serverId int
	peerIds  []int

	cm       *ConsensusModule
	rpcProxy *RPCProxy

	rpcServer *rpc.Server
	listener  net.Listener

	commitChan  chan<- CommitEntry
	peerClients map[int]*rpc.Client
	knownPeers  sync.Map // a map from the ID (int) to the address (net.Addr) of known peers

	ready <-chan any
	quit  chan any
	wg    sync.WaitGroup
}

func NewServer(serverId int, peerIds []int, ready <-chan any, commitChan chan<- CommitEntry) *Server {
	s := new(Server)
	s.serverId = serverId
	s.peerIds = peerIds
	s.peerClients = make(map[int]*rpc.Client)
	s.ready = ready
	s.commitChan = commitChan
	s.quit = make(chan any)
	return s
}

// Submit sends a command to the ConsensusModule for processing. It returns true
// if this server is the leader and therefore the command was submitted successfully,
// or false if this server is not the leader.
func (s *Server) Submit(command any) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cm.Submit(command)
}

func (s *Server) Report() (id, term int, isLeader bool, logEntries []LogEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cm.Report()
}

func (s *Server) Serve(listenAddr string) {
	s.mu.Lock()
	s.cm = NewConsensusModule(s.serverId, s.peerIds, s, s.ready, s.commitChan)

	// Create a new RPC server and register a RPCProxy that forwards all methods
	// to n.cm
	s.rpcServer = rpc.NewServer()
	s.rpcProxy = &RPCProxy{cm: s.cm}
	s.rpcServer.RegisterName("ConsensusModule", s.rpcProxy)

	var err error
	s.listener, err = net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("[%v] listening at %s", s.serverId, s.listener.Addr())
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		for {
			conn, err := s.listener.Accept()
			if err != nil {
				select {
				case <-s.quit:
					return
				default:
					log.Fatal("accept error:", err)
				}
			}
			s.wg.Add(1)
			go func() {
				s.rpcServer.ServeConn(conn)
				s.wg.Done()
			}()
		}
	}()
}

// DisconnectAll closes all the client connections to peers for this server.
func (s *Server) DisconnectAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.peerClients {
		if s.peerClients[id] != nil {
			s.peerClients[id].Close()
			s.peerClients[id] = nil
		}
	}
}

// Shutdown closes the server and waits for it to shut down properly.
func (s *Server) Shutdown() {
	s.cm.Stop()
	close(s.quit)
	s.listener.Close()
	s.wg.Wait()
}

func (s *Server) GetListenAddr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listener.Addr()
}

func (s *Server) ConnectToPeer(peerId int, addr net.Addr) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.knownPeers.Load(peerId); !ok {
		s.knownPeers.Store(peerId, addr)
	}
	if s.peerClients[peerId] == nil {
		client, err := rpc.Dial(addr.Network(), addr.String())
		if err != nil {
			return err
		}
		s.peerClients[peerId] = client
	}
	return nil
}

// DisconnectPeer disconnects this server from the peer identified by peerId.
func (s *Server) DisconnectPeer(peerId int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.peerClients[peerId] != nil {
		err := s.peerClients[peerId].Close()
		s.peerClients[peerId] = nil
		return err
	}
	return nil
}

func (s *Server) Call(id int, serviceMethod string, args any, reply any) error {
	s.mu.Lock()
	peer := s.peerClients[id]
	s.mu.Unlock()

	// If this is called after shutdown (where client.Close is called), it will
	// return an error.
	if peer == nil {
		return fmt.Errorf("call client %d after it's closed", id)
	}

	err := peer.Call(serviceMethod, args, reply)
	if err != nil {
		log.Printf("peer %d is not responding - launching a thread to try to reconnect", id)
		go func(peerId int, maxRetries int) {
			if err := s.reconnectToPeer(peerId, maxRetries); err != nil {
				log.Printf("failed to reconnect to peer %d, error %s", peerId, err)
				return
			}
			log.Printf("successfully reconnected to peer %d!", peerId)
		}(id, maxReconnectRetries)
	}

	return err
}

// reconnectToPeer disconnects from the peer with the given ID if a connection exists
// and keeps trying to reconnect with exponential backoff until the given maximum retries
// are reached or an error different than connection refused is received. In such case,
// this function will return a non-nil error.
func (s *Server) reconnectToPeer(peerId int, maxRetries int) error {
	s.DisconnectPeer(peerId)

	err := retry.Do(
		func() error {
			peerEntry, ok := s.knownPeers.Load(peerId)
			if !ok {
				return retry.Unrecoverable(fmt.Errorf("peer with ID %d is unknown", peerId))
			}

			peerAddr, ok := peerEntry.(net.Addr)
			if !ok {
				return retry.Unrecoverable(fmt.Errorf("peer with ID %d has an unexpected address format stored", peerId))
			}

			return s.ConnectToPeer(peerId, peerAddr)
		},
		retry.Attempts(uint(maxRetries)),
		retry.Delay(2*time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.RetryIf(func(err error) bool {
			return errors.Is(err, syscall.ECONNREFUSED)
		}),
		retry.OnRetry(func(n uint, err error) {
			log.Printf("reconnect retry #%d failed: %s", n, err)
		}),
	)

	return err
}

// RPCProxy is a pass-thru proxy server for ConsensusModule's RPC methods. It
// serves RPC requests made to a CM and manipulates them before forwarding to
// the CM itself.
//
// It's useful for things like:
//   - Simulating a small delay in RPC transmission.
//   - Simulating possible unreliable connections by delaying some messages
//     significantly and dropping others when RAFT_UNRELIABLE_RPC is set.
type RPCProxy struct {
	cm *ConsensusModule
}

func (rpp *RPCProxy) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) error {
	if len(os.Getenv("RAFT_UNRELIABLE_RPC")) > 0 {
		dice := rand.Intn(10)
		if dice == 9 {
			rpp.cm.dlog("drop RequestVote")
			return fmt.Errorf("RPC failed")
		} else if dice == 8 {
			rpp.cm.dlog("delay RequestVote")
			time.Sleep(75 * time.Millisecond)
		}
	} else {
		time.Sleep(time.Duration(1+rand.Intn(5)) * time.Millisecond)
	}
	return rpp.cm.RequestVote(args, reply)
}

func (rpp *RPCProxy) AppendEntries(args AppendEntriesArgs, reply *AppendEntriesReply) error {
	if len(os.Getenv("RAFT_UNRELIABLE_RPC")) > 0 {
		dice := rand.Intn(10)
		if dice == 9 {
			rpp.cm.dlog("drop AppendEntries")
			return fmt.Errorf("RPC failed")
		} else if dice == 8 {
			rpp.cm.dlog("delay AppendEntries")
			time.Sleep(75 * time.Millisecond)
		}
	} else {
		time.Sleep(time.Duration(1+rand.Intn(5)) * time.Millisecond)
	}
	return rpp.cm.AppendEntries(args, reply)
}
