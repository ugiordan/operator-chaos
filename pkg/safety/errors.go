package safety

import "errors"

// Sentinel errors for lock operations.
var (
	// ErrHolderMismatch indicates the lock is held by a different experiment.
	ErrHolderMismatch = errors.New("holder mismatch")
	// ErrLockNotFound indicates the lock does not exist (expired or never created).
	ErrLockNotFound = errors.New("lock not found")
	// ErrLockContention indicates the lock is held by another experiment during acquisition.
	ErrLockContention = errors.New("lock contention")
)
