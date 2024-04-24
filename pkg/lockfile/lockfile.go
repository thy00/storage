package lockfile

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// A Locker represents a file lock where the file is used to cache an
// identifier of the last party that made changes to whatever's being protected
// by the lock.
//
// Most users can refer to *LockFile, the provided implementation, instead.
type Locker interface {
	// Acquire a writer lock.
	// The default unix implementation panics if:
	// - opening the lockfile failed
	// - tried to lock a read-only lock-file
	Lock()

	// Acquire a writer lock recursively, allowing for recursive acquisitions
	// within the same process space.
	RecursiveLock()

	// Unlock the lock.
	// The default unix implementation panics if:
	// - unlocking an unlocked lock
	// - if the lock counter is corrupted
	Unlock()

	// Acquire a reader lock.
	RLock()

	// Touch records, for others sharing the lock, that the caller was the
	// last writer.  It should only be called with the lock held.
	//
	// Deprecated: Use *LockFile.RecordWrite.
	Touch() error

	// Modified() checks if the most recent writer was a party other than the
	// last recorded writer.  It should only be called with the lock held.
	// Deprecated: Use *LockFile.ModifiedSince.
	Modified() (bool, error)

	// TouchedSince() checks if the most recent writer modified the file (likely using Touch()) after the specified time.
	TouchedSince(when time.Time) bool

	// IsReadWrite() checks if the lock file is read-write
	IsReadWrite() bool

	// Locked() checks if lock is locked for writing by a thread in this process
	Locked() bool

	// AssertLocked() can be used by callers that _know_ that they hold the lock (for reading or writing), for sanity checking.
	// It might do nothing at all, or it may panic if the caller is not the owner of this lock.
	AssertLocked()

	// AssertLocked() can be used by callers that _know_ that they hold the lock locked for writing, for sanity checking.
	// It might do nothing at all, or it may panic if the caller is not the owner of this lock for writing.
	AssertLockedForWriting()
}

var (
	lockFiles     map[string]*LockFile
	lockFilesLock sync.Mutex
)

// GetLockFile opens a read-write lock file, creating it if necessary.  The
// *LockFile object may already be locked if the path has already been requested
// by the current process.
func GetLockFile(path string) (*LockFile, error) {
	return getLockfile(path, false)
}

// GetLockfile opens a read-write lock file, creating it if necessary.  The
// Locker object may already be locked if the path has already been requested
// by the current process.
//
// Deprecated: Use GetLockFile
func GetLockfile(path string) (Locker, error) {
	return getLockfile(path, false)
}

// GetROLockFile opens a read-only lock file, creating it if necessary.  The
// *LockFile object may already be locked if the path has already been requested
// by the current process.
func GetROLockFile(path string) (*LockFile, error) {
	return getLockfile(path, true)
}

// GetROLockfile opens a read-only lock file, creating it if necessary.  The
// Locker object may already be locked if the path has already been requested
// by the current process.
//
// Deprecated: Use GetROLockFile
func GetROLockfile(path string) (Locker, error) {
	return GetROLockFile(path)
}

// getLockFile returns a *LockFile object, possibly (depending on the platform)
// working inter-process, and associated with the specified path.
//
// If ro, the lock is a read-write lock and the returned *LockFile should correspond to the
// “lock for reading” (shared) operation; otherwise, the lock is either an exclusive lock,
// or a read-write lock and *LockFile should correspond to the “lock for writing” (exclusive) operation.
//
// WARNING:
// - The lock may or MAY NOT be inter-process.
// - There may or MAY NOT be an actual object on the filesystem created for the specified path.
// - Even if ro, the lock MAY be exclusive.
func getLockfile(path string, ro bool) (*LockFile, error) {
	lockFilesLock.Lock()
	defer lockFilesLock.Unlock()
	if lockFiles == nil {
		lockFiles = make(map[string]*LockFile)
	}
	cleanPath, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.Wrapf(err, "error ensuring that path %q is an absolute path", path)
	}
	if lockFile, ok := lockFiles[cleanPath]; ok {
		if ro && lockFile.IsReadWrite() {
			return nil, errors.Errorf("lock %q is not a read-only lock", cleanPath)
		}
		if !ro && !lockFile.IsReadWrite() {
			return nil, errors.Errorf("lock %q is not a read-write lock", cleanPath)
		}
		return lockFile, nil
	}
	lockFile, err := createLockFileForPath(cleanPath, ro) // platform-dependent locker
	if err != nil {
		return nil, err
	}
	lockFiles[cleanPath] = lockFile
	return lockFile, nil
}
