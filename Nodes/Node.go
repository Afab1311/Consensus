package main

import (
	"context"
	"log"
	"net"
	"os"
	"sync"
	"time"

	pb "github.com/afab1311/Consensus/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type DeferredRequest struct {
	nodeID string
	respCh chan *pb.NodeResponse
}

type Node struct {
	pb.UnimplementedConsensusServiceServer
	id           string
	addr         string
	peers        map[string]string
	clients      map[string]pb.ConsensusServiceClient
	mutex        sync.Mutex
	clock        int32
	request      bool
	requestTime  int32
	replies      map[string]bool
	deferredList []DeferredRequest
}

func NewNode(id, addr string, peers map[string]string) *Node {
	return &Node{
		id:           id,
		addr:         addr,
		peers:        peers,
		clients:      make(map[string]pb.ConsensusServiceClient),
		clock:        0,
		request:      false,
		replies:      make(map[string]bool),
		deferredList: make([]DeferredRequest, 0),
	}
}

func (n *Node) Start() error {
	lis, err := net.Listen("tcp", n.addr)
	if err != nil {
		return err
	}
	s := grpc.NewServer()
	pb.RegisterConsensusServiceServer(s, n)

	log.Printf("Node %s is starting server on %s", n.id, n.addr)
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	return nil
}

func (n *Node) ConnectToOthers() {
	time.Sleep(2 * time.Second)

	for otherID, otherAddr := range n.peers {
		conn, err := grpc.NewClient(otherAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("Node %s failed to connect to peer %s: %v", n.id, otherID, err)
			continue
		}
		n.mutex.Lock()
		n.clients[otherID] = pb.NewConsensusServiceClient(conn)
		n.mutex.Unlock()
		log.Printf("Node %s Connected to peer %s at %s", n.id, otherID, otherAddr)
	}
}

func (n *Node) Tick() int32 {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.clock++
	return n.clock
}

func (n *Node) UpdateClock(timestamp int32) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	if timestamp > n.clock {
		n.clock = timestamp
	}
	n.clock++
}

func (n *Node) RequestAccess() bool {
	timestamp := n.Tick()

	n.mutex.Lock()
	n.request = true
	n.requestTime = timestamp
	n.replies = make(map[string]bool)
	n.mutex.Unlock()

	log.Printf("Node %s is requesting access to CS with timestamp %d", n.id, timestamp)

	for pid, client := range n.clients {
		go func(pid string, cli pb.ConsensusServiceClient) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			resp, err := cli.SendRequest(ctx, &pb.NodeRequest{
				Response: n.id,
				Time:     int64(timestamp),
			})
			if err != nil {
				log.Printf("Node %s Error contacting %s: %v", n.id, pid, err)
				return
			}

			n.UpdateClock(int32(resp.Time))

			if resp.Response == "GRANT" {
				n.mutex.Lock()
				n.replies[pid] = true
				log.Printf("Node %s Received GRANT from %s",
					n.id, pid)
				n.mutex.Unlock()
			} else {
				log.Printf("Node %s recieved 'Defer' from %s. Waiting for access", n.id, pid)
			}
		}(pid, client)
	}

	for {
		n.mutex.Lock()
		allGranted := len(n.replies) == len(n.clients)
		n.mutex.Unlock()

		if allGranted {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("Node %s recieved GRANT from all Nodes, entering CS", n.id)
	return true
}

func (n *Node) SendRequest(ctx context.Context, req *pb.NodeRequest) (*pb.NodeResponse, error) {
	n.UpdateClock(int32(req.Time))

	log.Printf("Node %s received request from Node %s with timestamp %d (my clock: %d)",
		n.id, req.Response, req.Time, n.clock)

	n.mutex.Lock()

	shouldDefer := n.request &&
		(n.requestTime < int32(req.Time) ||
			(n.requestTime == int32(req.Time) && n.id < req.Response))

	if shouldDefer {
		respCh := make(chan *pb.NodeResponse, 1)
		n.deferredList = append(n.deferredList, DeferredRequest{
			nodeID: req.Response,
			respCh: respCh,
		})
		log.Printf("Node %s is deffering reply to Node %s (my request: %d, their request: %d)",
			n.id, req.Response, n.requestTime, req.Time)

		currentClock := n.clock
		n.mutex.Unlock()

		select {
		case resp := <-respCh:
			return resp, nil
		case <-ctx.Done():
			return &pb.NodeResponse{Response: "GRANT", Time: int64(currentClock)}, nil
		}
	}

	currentClock := n.clock
	n.mutex.Unlock()

	// Grant immediately
	log.Printf("Node %s is sending GRANT to Node %s", n.id, req.Response)
	return &pb.NodeResponse{Response: "GRANT", Time: int64(currentClock)}, nil
}

func (n *Node) EnterCS() {
	log.Printf("Node %s entering CS", n.id)
	log.Printf("Node %s is performing critical operation...", n.id)
	time.Sleep(2 * time.Second)
	log.Printf("Node %s exiting CS", n.id)
}

func (n *Node) ReleaseCS() {
	n.mutex.Lock()
	n.request = false
	deferred := make([]DeferredRequest, len(n.deferredList))
	copy(deferred, n.deferredList)
	n.deferredList = make([]DeferredRequest, 0)
	currentClock := n.clock
	n.mutex.Unlock()

	log.Printf("Node %s is releasing CS, sending deferred replies to %d nodes",
		n.id, len(deferred))

	for _, def := range deferred {
		log.Printf("Node %s i sending deferred GRANT to Node %s", n.id, def.nodeID)
		select {
		case def.respCh <- &pb.NodeResponse{Response: "GRANT", Time: int64(currentClock)}:
		default:
			log.Printf("Node %s Warning: could not send deferred reply to %s", n.id, def.nodeID)
		}
	}
}

func main() {
	file, err := os.Create("log.txt")
	if err != nil {
		log.Fatal(err)
	}
	log.SetOutput(file)

	if len(os.Args) < 3 {
		log.Fatal("Usage: go run node.go <node_id> <address>")
	}

	nodeID := os.Args[1]
	addr := os.Args[2]

	allNodes := map[string]string{
		"Node1": "localhost:5001",
		"Node2": "localhost:5002",
		"Node3": "localhost:5003",
		"Node4": "localhost:5004",
	}

	peers := make(map[string]string)
	for id, address := range allNodes {
		if id != nodeID {
			peers[id] = address
		}
	}

	node := NewNode(nodeID, addr, peers)

	if err := node.Start(); err != nil {
		log.Fatalf("Failed to start node: %v", err)
	}

	node.ConnectToOthers()

	time.Sleep(time.Duration(3+len(nodeID)) * time.Second)

	for i := 0; i < 3; i++ {
		node.RequestAccess()
		node.EnterCS()
		node.ReleaseCS()
		time.Sleep(3 * time.Second)
	}

	select {}
}
