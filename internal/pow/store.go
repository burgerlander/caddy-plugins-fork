package pow

import (
	"sync"
	"time"

	"github.com/tilinna/clock"
)

// Store is used to track information related to proof-of-work challenges and
// solutions.
type Store interface {

	// SetSolution stores that the given solution is valid for the seed. The
	// seed/solution combination will be cleared from the Store once the expiry
	// is reached.
	SetSolution(seed, solution []byte, expiresAt time.Time) error

	// IsSolution returns true if SetSolution has been called with the given
	// seed, and the expiry from that call has not yet elapsed.
	IsSolution(seed, solution []byte) bool

	Close() error
}

// MemoryStoreOpts are optional parameters to NewMemoryStore. A nil value is
// equivalent to a zero value.
type MemoryStoreOpts struct {
	// Clock is used for controlling the view of time.
	//
	// Defaults to clock.Realtime().
	Clock clock.Clock
}

func (o *MemoryStoreOpts) withDefaults() *MemoryStoreOpts {
	if o == nil {
		o = new(MemoryStoreOpts)
	}

	if o.Clock == nil {
		o.Clock = clock.Realtime()
	}

	return o
}

type memStoreKey struct {
	seed, solution string
}

type inMemStore struct {
	opts *MemoryStoreOpts

	m          map[memStoreKey]time.Time
	l          sync.RWMutex
	closeCh    chan struct{}
	spinLoopCh chan struct{} // only used by tests
}

const inMemStoreGCPeriod = 5 * time.Second

// NewMemoryStore initializes and returns an in-memory Store implementation.
func NewMemoryStore(opts *MemoryStoreOpts) Store {
	s := &inMemStore{
		opts:       opts.withDefaults(),
		m:          map[memStoreKey]time.Time{},
		closeCh:    make(chan struct{}),
		spinLoopCh: make(chan struct{}, 1),
	}
	go s.spin(s.opts.Clock.NewTicker(inMemStoreGCPeriod))
	return s
}

func (s *inMemStore) spin(ticker *clock.Ticker) {
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := s.opts.Clock.Now()

			s.l.Lock()
			for key, expiresAt := range s.m {
				if !now.Before(expiresAt) {
					delete(s.m, key)
				}
			}
			s.l.Unlock()

		case <-s.closeCh:
			return
		}

		select {
		case s.spinLoopCh <- struct{}{}:
		default:
		}
	}
}

func (s *inMemStore) SetSolution(
	seed, solution []byte, expiresAt time.Time,
) error {
	key := memStoreKey{
		seed:     string(seed),
		solution: string(solution),
	}

	s.l.Lock()
	defer s.l.Unlock()

	s.m[key] = expiresAt
	return nil
}

func (s *inMemStore) IsSolution(seed, solution []byte) bool {
	key := memStoreKey{
		seed:     string(seed),
		solution: string(solution),
	}

	s.l.RLock()
	defer s.l.RUnlock()

	expiresAt, ok := s.m[key]
	return ok && expiresAt.After(s.opts.Clock.Now())
}

func (s *inMemStore) Close() error {
	close(s.closeCh)
	return nil
}
