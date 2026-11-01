//go:build unix

package lock

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestLockExcludesSecondHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")
	first, err := Acquire(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Acquire(path, 20*time.Millisecond)
	if !aerr.HasCode(err, aerr.StateLockTimeout) {
		t.Fatalf("second Acquire err = %v, want E-STATE-LOCK-TIMEOUT", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	second, err := Acquire(path, time.Second)
	if err != nil {
		t.Fatalf("Acquire after release: %v", err)
	}
	if err := second.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestLockWaitsForRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")
	first, err := Acquire(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(30 * time.Millisecond)
		_ = first.Release()
	}()
	second, err := Acquire(path, 2*time.Second)
	if err != nil {
		t.Fatalf("blocked Acquire did not get the lock after release: %v", err)
	}
	if err := second.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestLockErrors(t *testing.T) {
	if _, err := Acquire(filepath.Join(t.TempDir(), "missing", ".lock"), time.Second); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("missing dir: err = %v, want E-STATE-IO", err)
	}
	l, err := Acquire(filepath.Join(t.TempDir(), ".lock"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
	if err := l.Release(); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("double release: err = %v, want E-STATE-IO", err)
	}
}

// TestConcurrentWritersSerialised runs read-modify-write cycles under the lock from many
// goroutines; without mutual exclusion increments would be lost.
func TestConcurrentWritersSerialised(t *testing.T) {
	dir := t.TempDir()
	lockPath, counter := filepath.Join(dir, ".lock"), filepath.Join(dir, "counter")
	if err := os.WriteFile(counter, []byte("0"), 0o600); err != nil {
		t.Fatal(err)
	}
	const workers, rounds = 8, 20
	var wg sync.WaitGroup
	errs := make(chan error, workers*rounds)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range rounds {
				errs <- increment(lockPath, counter)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(counter)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := strconv.Atoi(string(b)); got != workers*rounds {
		t.Errorf("counter = %d, want %d", got, workers*rounds)
	}
}

func increment(lockPath, counter string) error {
	l, err := Acquire(lockPath, 5*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = l.Release() }()
	b, err := os.ReadFile(counter)
	if err != nil {
		return err
	}
	n, err := strconv.Atoi(string(b))
	if err != nil {
		return err
	}
	return os.WriteFile(counter, []byte(strconv.Itoa(n+1)), 0o600)
}
