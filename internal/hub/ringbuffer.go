package hub

import (
	"sync"
)

// RingBuffer stores a fixed-capacity circular buffer of Events.
// It is thread-safe and retains the most recent N elements without growing memory.
type RingBuffer struct {
	mu       sync.RWMutex
	capacity int
	items    []Event
	start    int
	count    int
}

// NewRingBuffer creates a ring buffer with given maximum capacity
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 1000
	}
	return &RingBuffer{
		capacity: capacity,
		items:    make([]Event, capacity),
	}
}

// Push adds an event to the buffer, evicting the oldest if capacity is reached
func (r *RingBuffer) Push(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.count < r.capacity {
		idx := (r.start + r.count) % r.capacity
		r.items[idx] = e
		r.count++
	} else {
		// Overwrite the oldest element at r.start
		r.items[r.start] = e
		r.start = (r.start + 1) % r.capacity
	}
}

// GetAll returns all items in chronological order (oldest to newest)
func (r *RingBuffer) GetAll() []Event {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Event, r.count)
	for i := 0; i < r.count; i++ {
		idx := (r.start + i) % r.capacity
		result[i] = r.items[idx]
	}
	return result
}

// GetLast returns the last n items in chronological order
func (r *RingBuffer) GetLast(n int) []Event {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if n <= 0 || r.count == 0 {
		return nil
	}
	if n > r.count {
		n = r.count
	}

	result := make([]Event, n)
	offset := r.count - n
	for i := 0; i < n; i++ {
		idx := (r.start + offset + i) % r.capacity
		result[i] = r.items[idx]
	}
	return result
}

// Len returns current number of stored elements
func (r *RingBuffer) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.count
}
