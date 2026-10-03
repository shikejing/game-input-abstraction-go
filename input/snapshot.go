package input

import "sync"

// SnapshotBus is a conflating frame channel: producers publish frames without
// ever blocking, and consumers always observe the latest consistent snapshot.
// Slow consumers never stall the sampling goroutine; intermediate frames are
// simply skipped.
type SnapshotBus struct {
	mu     sync.Mutex
	cond   *sync.Cond
	latest Frame
	has    bool
	seq    uint64
}

// NewSnapshotBus creates an empty snapshot bus.
func NewSnapshotBus() *SnapshotBus {
	b := &SnapshotBus{}
	b.cond = sync.NewCond(&b.mu)
	return b
}

// Publish stores a new frame. It never blocks.
func (b *SnapshotBus) Publish(f Frame) {
	b.mu.Lock()
	b.latest = f
	b.has = true
	b.seq++
	b.cond.Broadcast()
	b.mu.Unlock()
}

// Latest returns a deep copy of the most recent frame and whether a frame has
// ever been published. The copy is safe to read while the sampler continues.
func (b *SnapshotBus) Latest() (Frame, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.has {
		return Frame{}, false
	}
	return b.latest.Clone(), true
}

// Wait blocks until a frame newer than afterSeq is published, then returns a
// deep copy of it. afterSeq should be the Seq of the last observed frame (0
// to wait for any frame).
func (b *SnapshotBus) Wait(afterSeq uint64) (Frame, uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for !b.has || b.seq <= afterSeq {
		b.cond.Wait()
	}
	return b.latest.Clone(), b.seq
}

// Seq returns the sequence number of the latest published frame.
func (b *SnapshotBus) Seq() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.seq
}
