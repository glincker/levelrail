package platformimport

import (
	"errors"
	"fmt"
	"io"
	"sync/atomic"
)

// ErrTooLarge marks an import source that went past the live size or item caps.
var ErrTooLarge = errors.New("import source is too large")

const defaultMaxItems = 5000

// The live import.max_bytes and import.max_items settings land here; zero means the built-in default.
var (
	maxBytesLive atomic.Int64
	maxItemsLive atomic.Int64
)

// SetLimits applies the import.max_bytes and import.max_items settings; a value of 0 restores the default.
func SetLimits(maxBytes int64, maxItems int) {
	maxBytesLive.Store(maxBytes)
	maxItemsLive.Store(int64(maxItems))
}

// MaxBytes is the largest source payload read from one platform call or host scan.
func MaxBytes() int64 {
	if n := maxBytesLive.Load(); n > 0 {
		return n
	}
	return maxResponseBytes
}

// MaxItems is the most apps or containers one discovery may return.
func MaxItems() int {
	if n := maxItemsLive.Load(); n > 0 {
		return int(n)
	}
	return defaultMaxItems
}

// ReadBounded reads r fully but never more than MaxBytes, failing instead of truncating.
func ReadBounded(r io.Reader, what string) ([]byte, error) {
	limit := MaxBytes()
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", what, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes: %w", what, limit, ErrTooLarge)
	}
	return data, nil
}

// CheckBytes rejects an already-read payload over the byte cap.
func CheckBytes(n int, what string) error {
	if int64(n) > MaxBytes() {
		return fmt.Errorf("%s exceeds %d bytes: %w", what, MaxBytes(), ErrTooLarge)
	}
	return nil
}

// CheckItems rejects a discovery with more entries than the item cap.
func CheckItems(n int, what string) error {
	if n > MaxItems() {
		return fmt.Errorf("%s has %d entries, over the limit of %d: %w", what, n, MaxItems(), ErrTooLarge)
	}
	return nil
}
