package client

import (
	"sync"

	"github.com/lambdcalculus/scs/internal/uid"
)

// Implements a list of clients with a set data structure.
type List struct {
	// set data structure: https://gist.github.com/bgadrian/cb8b9344d9c66571ef331a14eb7a2e80
	set map[*Client]struct{}
	mu  sync.Mutex
}

// Creates a new client list.
func NewList() *List {
	return &List{set: make(map[*Client]struct{})}
}

// Adds a client to the list.
func (l *List) Add(c *Client) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.set[c] = struct{}{}
}

// Removes a client to the list.
func (l *List) Remove(c *Client) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.set, c)
}

// Returns a copy of the underlying set, which can be ranged over.
func (l *List) Clients() map[*Client]struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	cpy := make(map[*Client]struct{})
	for c := range l.set {
		cpy[c] = struct{}{}
	}
	return cpy
}

// Returns the set of joined clients, which can be ranged over.
func (l *List) ClientsJoined() map[*Client]struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	cpy := make(map[*Client]struct{})
	for c := range l.set {
		if c.Joined() {
			cpy[c] = struct{}{}
		}
	}
	return cpy
}

func (l *List) Size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	count := 0
	for range l.set {
		count++
	}
	return count
}

func (l *List) SizeJoined() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	count := 0
	for c := range l.set {
		if c.uid != uid.Unjoined {
			count++
		}
	}
	return count
}
