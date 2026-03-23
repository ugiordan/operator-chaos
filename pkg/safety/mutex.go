package safety

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ExperimentLock prevents concurrent experiments from running against the same operator.
type ExperimentLock interface {
	Acquire(ctx context.Context, operator string, experimentName string, leaseDuration time.Duration) error
	Renew(ctx context.Context, operator string, experimentName string) error
	Release(ctx context.Context, operator string, experimentName string) error
}

type localExperimentLock struct {
	mu    sync.Mutex
	locks map[string]string // operator -> experimentName
}

// NewLocalExperimentLock returns an in-process ExperimentLock backed by a sync.Mutex.
func NewLocalExperimentLock() ExperimentLock {
	return &localExperimentLock{
		locks: make(map[string]string),
	}
}

func (l *localExperimentLock) Acquire(_ context.Context, operator string, experimentName string, _ time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if existing, ok := l.locks[operator]; ok {
		// Self-re-acquire: if same experiment already holds the lock, allow it.
		if existing == experimentName {
			return nil
		}
		return fmt.Errorf("operator %s is locked by experiment %q: %w", operator, existing, ErrLockContention)
	}

	l.locks[operator] = experimentName
	return nil
}

func (l *localExperimentLock) Renew(_ context.Context, operator string, experimentName string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	existing, ok := l.locks[operator]
	if !ok {
		return fmt.Errorf("no lock held for operator %s: %w", operator, ErrLockNotFound)
	}
	if existing != experimentName {
		return fmt.Errorf("lock held by %q, renew requested by %q: %w", existing, experimentName, ErrHolderMismatch)
	}
	return nil
}

func (l *localExperimentLock) Release(_ context.Context, operator string, experimentName string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	existing, ok := l.locks[operator]
	if !ok {
		return fmt.Errorf("no lock for operator %s: %w", operator, ErrLockNotFound)
	}
	if existing != experimentName {
		return fmt.Errorf("lock held by %q, release requested by %q: %w", existing, experimentName, ErrHolderMismatch)
	}
	delete(l.locks, operator)
	return nil
}
