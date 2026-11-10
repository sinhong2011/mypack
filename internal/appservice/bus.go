package appservice

import "sync"

// bus is a non-blocking event fan-out. Every subscriber owns a buffered queue
// drained by its own goroutine; when a slow consumer's queue is full the
// oldest event is dropped, so publishers never block (the GUI re-reads state
// from the service, so a dropped event only costs a redundant refresh).
type bus struct {
	mu     sync.Mutex
	subs   map[int]*subscriber
	next   int
	closed bool
}

const subscriberBuffer = 256

type subscriber struct {
	ch   chan Event
	done chan struct{}
}

func newBus() *bus { return &bus{subs: map[int]*subscriber{}} }

func (b *bus) subscribe(fn func(Event)) func() {
	s := &subscriber{ch: make(chan Event, subscriberBuffer), done: make(chan struct{})}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return func() {}
	}
	id := b.next
	b.next++
	b.subs[id] = s
	b.mu.Unlock()

	go func() {
		defer close(s.done)
		for e := range s.ch {
			fn(e)
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			if _, ok := b.subs[id]; ok {
				delete(b.subs, id)
				close(s.ch)
			}
			b.mu.Unlock()
		})
	}
}

func (b *bus) publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, s := range b.subs {
		select {
		case s.ch <- e:
		default:
			// Queue full: drop the oldest event, then enqueue the new one.
			select {
			case <-s.ch:
			default:
			}
			select {
			case s.ch <- e:
			default:
			}
		}
	}
}

// close ends all subscriptions; later publishes are no-ops.
func (b *bus) close() {
	b.mu.Lock()
	b.closed = true
	subs := b.subs
	b.subs = map[int]*subscriber{}
	for _, s := range subs {
		close(s.ch)
	}
	b.mu.Unlock()
}
