package lock

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gofrs/flock"
)

// Unlocker releases held locks.
type Unlocker struct {
	locks []*flock.Flock
}

// Release releases all held locks. Safe to call multiple times.
func (u *Unlocker) Release() error {
	var firstErr error
	// Release in reverse order
	for i := len(u.locks) - 1; i >= 0; i-- {
		if err := u.locks[i].Unlock(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	u.locks = nil
	return firstErr
}

// Acquire acquires exclusive locks on the given file paths.
// Uses sidecar .lock files (e.g. data.xhist.lock).
// Locks are acquired in alphabetical order to prevent deadlocks.
// Times out after 10 seconds with 100ms retry interval.
// Accepts 1 or more paths.
func Acquire(ctx context.Context, paths ...string) (*Unlocker, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("lock: no paths provided")
	}

	// Deduplicate and sort lock paths
	lockPaths := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, p := range paths {
		lp := p + ".lock"
		if !seen[lp] {
			seen[lp] = true
			lockPaths = append(lockPaths, lp)
		}
	}
	sort.Strings(lockPaths)

	lockCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var locks []*flock.Flock
	for _, path := range lockPaths {
		fl := flock.New(path)
		locked, err := fl.TryLockContext(lockCtx, 100*time.Millisecond)
		if err != nil {
			// Release already acquired locks
			for j := len(locks) - 1; j >= 0; j-- {
				locks[j].Unlock()
			}
			return nil, fmt.Errorf("lock %s: %w", path, err)
		}
		if !locked {
			for j := len(locks) - 1; j >= 0; j-- {
				locks[j].Unlock()
			}
			return nil, fmt.Errorf("lock %s: timeout", path)
		}
		locks = append(locks, fl)
	}

	return &Unlocker{locks: locks}, nil
}
