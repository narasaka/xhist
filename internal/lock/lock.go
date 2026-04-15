package lock

import (
	"context"
	"fmt"
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

// Acquire acquires exclusive locks on the xhist and xlsx files.
// Uses sidecar .lock files (e.g. data.xhist.lock, data.xlsx.lock).
// Locks are acquired in alphabetical order to prevent deadlocks.
// Times out after 10 seconds with 100ms retry interval.
func Acquire(ctx context.Context, xhistPath, xlsxPath string) (*Unlocker, error) {
	// Sort paths alphabetically for consistent ordering
	paths := sortPaths(xhistPath+".lock", xlsxPath+".lock")

	lockCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var locks []*flock.Flock
	for _, path := range paths {
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

func sortPaths(a, b string) []string {
	if a <= b {
		return []string{a, b}
	}
	return []string{b, a}
}
