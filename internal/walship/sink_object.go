package walship

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/objectstore"
)

// Bucket is the slice of objectstore.Client the sink needs.
type Bucket interface {
	Put(ctx context.Context, key string, body io.ReadSeeker, contentType, contentEncoding string) error
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
	List(ctx context.Context, prefix, token string, limit int32) (objectstore.ListPage, error)
}

const (
	objectListPage = 1000
	// DefaultObjectTimeout bounds one object-store call; callers override it from env.
	DefaultObjectTimeout = 2 * time.Minute
)

// ErrNoIdentity means a Get was attempted on a sink built without identities.
var ErrNoIdentity = errors.New("walship: sink has no age identity to decrypt with")

// ObjectSink stores each replica object as one age-encrypted S3 object under
// Prefix, so the bucket only ever sees ciphertext. The shipping side needs only
// Recipients; the standby adds Identities to read.
type ObjectSink struct {
	Bucket     Bucket
	Prefix     string
	Recipients []age.Recipient
	Identities []age.Identity
	// Timeout bounds each call; zero means DefaultObjectTimeout.
	Timeout time.Duration
	// TempDir holds ciphertext while uploading; empty uses the OS default.
	TempDir string
}

func (o *ObjectSink) key(name string) string { return o.Prefix + name }

func (o *ObjectSink) ctx() (context.Context, context.CancelFunc) {
	t := o.Timeout
	if t <= 0 {
		t = DefaultObjectTimeout
	}
	return context.WithTimeout(context.Background(), t)
}

// Put encrypts r to a temp file (the SDK needs a seekable body) and uploads it.
func (o *ObjectSink) Put(name string, r io.Reader) error {
	if len(o.Recipients) == 0 {
		return errors.New("walship: sink has no age recipient")
	}
	tmp, err := os.CreateTemp(o.TempDir, "walship-put-*")
	if err != nil {
		return fmt.Errorf("walship: temp: %w", err)
	}
	defer func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }()
	enc, err := age.Encrypt(tmp, o.Recipients...)
	if err != nil {
		return fmt.Errorf("walship: encrypt: %w", err)
	}
	if _, err := io.Copy(enc, r); err != nil {
		return fmt.Errorf("walship: encrypt %s: %w", name, err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("walship: encrypt %s: %w", name, err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("walship: rewind: %w", err)
	}
	ctx, cancel := o.ctx()
	defer cancel()
	return o.Bucket.Put(ctx, o.key(name), tmp, "application/octet-stream", "")
}

type decReader struct {
	io.Reader
	body io.Closer
	done context.CancelFunc
}

func (d decReader) Close() error { d.done(); return d.body.Close() }

// Get downloads and decrypts name. The timeout covers the whole read.
func (o *ObjectSink) Get(name string) (io.ReadCloser, error) {
	if len(o.Identities) == 0 {
		return nil, ErrNoIdentity
	}
	ctx, cancel := o.ctx()
	body, _, err := o.Bucket.Get(ctx, o.key(name))
	if err != nil {
		cancel()
		return nil, err
	}
	dec, err := age.Decrypt(body, o.Identities...)
	if err != nil {
		_ = body.Close()
		cancel()
		var noMatch *age.NoIdentityMatchError
		if errors.As(err, &noMatch) {
			return nil, fmt.Errorf("walship: %s: none of the supplied identities can decrypt it", name)
		}
		return nil, fmt.Errorf("walship: decrypt %s: %w", name, err)
	}
	return decReader{Reader: dec, body: body, done: cancel}, nil
}

// List returns object names (prefix stripped) that start with prefix, sorted.
func (o *ObjectSink) List(prefix string) ([]string, error) {
	var out []string
	token := ""
	for {
		ctx, cancel := o.ctx()
		page, err := o.Bucket.List(ctx, o.key(prefix), token, objectListPage)
		cancel()
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Objects {
			out = append(out, strings.TrimPrefix(obj.Key, o.Prefix))
		}
		if page.Next == "" {
			break
		}
		token = page.Next
	}
	sort.Strings(out)
	return out, nil
}

var _ Sink = (*ObjectSink)(nil)
