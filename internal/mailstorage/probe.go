package mailstorage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
)

// Step is the outcome of one probe operation.
type Step struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// ProbeResult reports every step so a read-only token is explained plainly.
type ProbeResult struct {
	OK    bool   `json:"ok"`
	Steps []Step `json:"steps"`
	// Err is the classified cause of the first failure.
	Err error `json:"-"`
}

// Probe runs put, get, list and delete of a tiny object under the
// service's probe prefix. After a failure it still tries to clean up.
func Probe(ctx context.Context, conn Connection) ProbeResult {
	res := ProbeResult{Steps: []Step{}}
	st, err := Open(ctx, conn)
	if err != nil {
		res.Steps = append(res.Steps, Step{Name: "open", Error: err.Error()})
		res.Err = err
		return res
	}
	payload := make([]byte, 16)
	if _, err := rand.Read(payload); err != nil {
		res.Err = err
		return res
	}
	key := ProbePrefix(conn.ServiceID) + hex.EncodeToString(payload)

	fail := func(name string, err error) ProbeResult {
		c := classify(name, err)
		res.Steps = append(res.Steps, Step{Name: name, Error: c.Error()})
		res.Err = c
		return res
	}
	if err := st.Put(ctx, key, bytes.NewReader(payload)); err != nil {
		return fail("put", err)
	}
	res.Steps = append(res.Steps, Step{Name: "put", OK: true})

	body, _, err := st.Get(ctx, key)
	if err != nil {
		return failAfterPut(st, fail, "get", err, key)
	}
	got, readErr := io.ReadAll(io.LimitReader(body, 1024))
	_ = body.Close()
	if readErr != nil || !bytes.Equal(got, payload) {
		if readErr == nil {
			readErr = io.ErrUnexpectedEOF
		}
		return failAfterPut(st, fail, "get", readErr, key)
	}
	res.Steps = append(res.Steps, Step{Name: "get", OK: true})

	page, err := st.List(ctx, ProbePrefix(conn.ServiceID), "", 10)
	if err != nil {
		return failAfterPut(st, fail, "list", err, key)
	}
	found := false
	for _, o := range page.Objects {
		found = found || o.Key == key
	}
	if !found {
		return failAfterPut(st, fail, "list", io.ErrUnexpectedEOF, key)
	}
	res.Steps = append(res.Steps, Step{Name: "list", OK: true})

	if err := st.Delete(ctx, key); err != nil {
		return fail("delete", err)
	}
	res.Steps = append(res.Steps, Step{Name: "delete", OK: true})
	res.OK = true
	return res
}

// failAfterPut removes the probe object best-effort before reporting.
func failAfterPut(st *Store, fail func(string, error) ProbeResult, step string, err error, key string) ProbeResult {
	_ = st.Delete(context.Background(), key)
	return fail(step, err)
}
