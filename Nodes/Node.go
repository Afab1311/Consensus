package main

import (
	"sync"

	pb ""
)

type Node struct {
	pb.UnimplementedMutexServer
	id      string
	addr    string
	peers   map[string]string
	clients map[string]pb.MutexClient
	mutex   sync.Mutex
}
