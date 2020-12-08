package recruiter

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"sort"
	"sync"
	"time"
)

// WorkerPool holds the internal state of the recruiter system.
// The methods for WorkerPool provide the core algorithms for a
// load-balancing API. Exported methods are thread safe.
type WorkerPool struct {
	mu           sync.Mutex                // one lock for all three maps
	freePool     map[string]WorkerDetail   // Map of all free workers
	nodePool     map[string][]WorkerDetail // Map of nodes of slices of free workers
	reservedPool map[string]WorkerDetail   // Map of busy workers
	SkipSpread   bool                      // option to test algorithm performance
	r            *rand.Rand
}

type WorkerDetail struct {
	Addr     string
	NodeName string
	Ctx      context.Context // context to cancel longpoll for recruited worker
}

func NewWorkerPool() *WorkerPool {
	return &WorkerPool{
		freePool:     make(map[string]WorkerDetail),
		nodePool:     make(map[string][]WorkerDetail),
		reservedPool: make(map[string]WorkerDetail),
		r:            rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Register adds workers to both the freePool and to the nodePool.
// Returns true when the worker has been added to the freePool.
// Returns false when the worker was reserved and was not added to the freePool.
// In the nodePool, workers are added to the end of the slice,
// and when they are recruited workers are removed from the start of the slice.
// So the []WorkerDetail slice for each node functions as a FIFO queue.
func (pool *WorkerPool) Register(addr string, nodename string, ctx context.Context) (bool, error) {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return false, fmt.Errorf("invalid address for Register: %w", err)
	}
	if nodename == "" {
		return false, fmt.Errorf("node name required for Register")
	}
	wd := WorkerDetail{Addr: addr, NodeName: nodename, Ctx: ctx}

	pool.mu.Lock()
	defer pool.mu.Unlock()
	if _, ok := pool.reservedPool[addr]; ok {
		return false, nil // if the worker is reserved, it will not be registered
	}
	pool.freePool[addr] = wd
	pool.nodePool[nodename] = append(pool.nodePool[nodename], wd)
	return true, nil
}

// removeFromNodePool is internal and the calling function must hold the lock.
func (pool *WorkerPool) removeFromNodePool(wd WorkerDetail) {
	s := pool.nodePool[wd.NodeName]
	if len(s) == 1 {
		if s[0] == wd {
			// Remove empty list from the hash so len(pool.nodePool)
			// is a a count of nodes with available workers.
			delete(pool.nodePool, wd.NodeName)
		}
		return
	}
	i := -1
	for j, v := range s {
		if v == wd {
			i = j
			break
		}
	}
	if i == -1 {
		panic(fmt.Errorf("expected WorkerDetail not in list: %v", wd.Addr))
	}
	// Overwrite the removed node and truncate the slice.
	s[i] = s[len(s)-1]
	pool.nodePool[wd.NodeName] = s[:len(s)-1]
}

func (pool *WorkerPool) Deregister(addr string) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if wd, ok := pool.freePool[addr]; ok {
		pool.removeFromNodePool(wd)
		delete(pool.freePool, addr)
	}
}

// Unreserve removes from the reserved pool, but does not reregister.
// The worker process should initiate register.
func (pool *WorkerPool) Unreserve(addrs []string) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	for _, addr := range addrs {
		delete(pool.reservedPool, addr)
	}
}

// Recruit attempts to return a set of workers distributed evenly
// across the maximum number of nodes.
func (pool *WorkerPool) Recruit(n int) ([]WorkerDetail, error) {
	if n < 1 {
		return nil, fmt.Errorf("recruit must request one or more workers: n=%d", n)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()
	if len(pool.nodePool) < 1 {
		return nil, nil
	}
	// Make a single pass through the nodes in the cluster that have
	// available workers, and try to pick evenly from each node. If that pass
	// fails to recruit enough workers, then we start to pick from the freePool,
	// regardless of node, until we recruit enough workers, or all available workers.
	var recruits []WorkerDetail
	if !pool.SkipSpread && n > 1 {
		var keys []string
		for k := range pool.nodePool {
			keys = append(keys, k)
		}
		pool.r.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
		for i, key := range keys {
			workers := pool.nodePool[key]
			// adjust goal on each iteration
			d := len(keys) - i
			goal := (n - len(recruits) + d - 1) / d
			if len(workers) > goal {
				recruits = append(recruits, workers[:goal]...)
				pool.nodePool[key] = workers[goal:]
			} else {
				recruits = append(recruits, workers...)
				delete(pool.nodePool, key)
			}
			if len(recruits) == n {
				break
			}
		}
		// Delete the recruits obtained in this pass from the freePool
		for _, wd := range recruits {
			if _, ok := pool.freePool[wd.Addr]; !ok {
				panic(fmt.Errorf("attempt to remove addr that was not in freePool: %v", wd.Addr))
			}
			delete(pool.freePool, wd.Addr)
		}
	}
	// If there are still recruits needed, select them by iterating through the freePool
	if len(recruits) < n {
		for k, wd := range pool.freePool {
			pool.removeFromNodePool(wd)
			delete(pool.freePool, k)
			recruits = append(recruits, wd)
			if len(recruits) == n {
				break
			}
		}
	}
	// Add the recruits to the Reserved Pool
	for _, r := range recruits {
		pool.reservedPool[r.Addr] = r
	}
	return recruits, nil
}

func (pool *WorkerPool) LenFreePool() int {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	return len(pool.freePool)
}

func (pool *WorkerPool) LenReservedPool() int {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	return len(pool.reservedPool)
}

func (pool *WorkerPool) LenNodePool() int {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	return len(pool.nodePool)
}

func (pool *WorkerPool) ListFreePool() []WorkerDetail {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	var recruits []WorkerDetail
	var keys []string
	for k := range pool.freePool {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		recruits = append(recruits, pool.freePool[key])
	}
	return recruits
}
