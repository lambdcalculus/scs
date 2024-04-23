// Package `uid` implements a UID heap.
package uid

import (
	"sync"

	"github.com/lambdcalculus/scs/pkg/minheap"
)

// If a client is connected but hasn't joined, its UID should be 0.
const (
    Unjoined = 0
)

// The UIDHeap stores which UID values can be taken by new users.
// Its methods can be called from multiple goroutines.
type UIDHeap struct {
	heap minheap.MinHeap
	mu   sync.Mutex
}

// Creates a new [UIDHeap] that can give up to `max` UIDs (1, 2, ..., max).
func CreateHeap(max int) *UIDHeap {
	init := make([]int, max)
    for i := 0; i < max; i++ {
		init[i] = i+1
	}
	return &UIDHeap{
		heap: minheap.NewHeap(init),
	}
}

// Takes and returns the smallest available UID, popping it from the heap.
func (u *UIDHeap) Take() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.heap.Pop()
}

// Frees the passed UID, pushing it into the heap.
func (u *UIDHeap) Free(id int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.heap.Push(id)
}
