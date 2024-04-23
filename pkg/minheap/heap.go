// Package minheap implements an integer minheap.
package minheap

import (
    "container/heap"
)

// MinHeap provides the integer minheap functionality.
// It can be passed as a copy, as it works with pointers internally.
// It is not goroutine-safe, users must implement mutexes on their end.
type MinHeap struct {
    heapImpl *intHeap
}

type intHeap []int

// NewHeap makes a new [MinHeap] with the initial values from `init`.
func NewHeap(init []int) MinHeap {
    var ih intHeap
    if init != nil {
        ih = make(intHeap, len(init))
        copy(ih, init)
    } else {
        ih = intHeap{}
    }
    heap.Init(&ih)

    return MinHeap{heapImpl: &ih}
}

// Min returns the smallest element from a [MinHeap].
// The time complexity is O(1).
func (h MinHeap) Min() int {
    return (*h.heapImpl)[0]
}

// Pop pops the smallest element from a [MinHeap].
// The time complexity is O(log n)
func (h MinHeap) Pop() int {
    return heap.Pop(h.heapImpl).(int)
}

// Push pushes a new element into a [MinHeap].
// The time complexity is O(log n)
func (h MinHeap) Push(x int) {
    heap.Push(h.heapImpl, x)
}

// Below are the necessary methods for [heap.Interface].

func (h intHeap) Len() int { return len(h) }
func (h intHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h intHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *intHeap) Push(x any) {
    *h = append(*h, x.(int))
}

func (h *intHeap) Pop() any {
    // get last element
    last := (*h)[len(*h)-1]

    // remove last element
    *h = (*h)[0:len(*h)-1]

    return last
}
