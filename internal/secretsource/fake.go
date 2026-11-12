package secretsource

import (
	"context"
	"strconv"
	"sync"
)

// FakeProvider is an in-memory Provider for tests, including other packages.
type FakeProvider struct {
	mu     sync.Mutex
	name   string
	data   map[string]Value
	err    error
	calls  int
	caps   Capabilities
	denied map[string]bool
}

// NewFake returns an empty fake named name.
func NewFake(name string) *FakeProvider {
	return &FakeProvider{name: name, data: map[string]Value{}, denied: map[string]bool{}, caps: Capabilities{Versions: true}}
}

func fakeKey(path, key string, version int) string {
	return path + "#" + key + "@" + strconv.Itoa(version)
}

// Set stores the latest value (version 0) for path and key.
func (f *FakeProvider) Set(path, key, value string) *FakeProvider {
	return f.SetVersion(path, key, 0, value)
}

// SetVersion stores a specific version; version 0 is the latest.
func (f *FakeProvider) SetVersion(path, key string, version int, value string) *FakeProvider {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[fakeKey(path, key, version)] = NewValue([]byte(value), strconv.Itoa(version))
	return f
}

// Deny makes path return ErrDenied.
func (f *FakeProvider) Deny(path string) {
	f.mu.Lock()
	f.denied[path] = true
	f.mu.Unlock()
}

// FailWith makes every Resolve return err (nil clears it).
func (f *FakeProvider) FailWith(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
}

// Calls reports how many times Resolve ran.
func (f *FakeProvider) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// Name implements Provider.
func (f *FakeProvider) Name() string { return f.name }

// Capabilities implements Provider.
func (f *FakeProvider) Capabilities() Capabilities { return f.caps }

// Resolve implements Provider.
func (f *FakeProvider) Resolve(_ context.Context, ref Ref) (Value, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return Value{}, f.err
	}
	if f.denied[ref.Path] {
		return Value{}, ErrDenied
	}
	v, ok := f.data[fakeKey(ref.Path, ref.Key, ref.Version)]
	if !ok {
		return Value{}, ErrNotFound
	}
	return v, nil
}
