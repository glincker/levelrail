package mailstorage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// AllowPrivateEnv opts in to private endpoints such as a self-hosted MinIO.
const AllowPrivateEnv = "APP_MAIL_STORAGE_ALLOW_PRIVATE"

const secretEnvKey = "SECRET_ACCESS_KEY" //nolint:gosec // slot name, not a credential

var serviceIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// SecretStore is the sealing surface of internal/secrets.Manager. Values
// are encrypted at rest and bound to the service slot they were written to.
type SecretStore interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	DeleteValue(ctx context.Context, serviceName, envKey string) error
}

// Spec is the write-only input for a connection. It carries the secret
// once and is redacted everywhere it could be printed.
type Spec struct {
	ServiceID   string
	Endpoint    string
	Region      string
	Bucket      string
	AccessKeyID string
	Secret      string
	PathStyle   bool
}

// String redacts the secret so a printed spec never leaks it.
func (s Spec) String() string {
	return fmt.Sprintf("{ServiceID:%s Bucket:%s Secret:[redacted]}", s.ServiceID, s.Bucket)
}

// GoString covers %#v with the same redaction.
func (s Spec) GoString() string { return "mailstorage.Spec" + s.String() }

// LogValue redacts the secret for slog.
func (s Spec) LogValue() slog.Value {
	return slog.GroupValue(slog.String("service_id", s.ServiceID), slog.String("bucket", s.Bucket), slog.String("secret", "[redacted]"))
}

// MarshalJSON never emits the secret.
func (s Spec) MarshalJSON() ([]byte, error) { return []byte(`{"redacted":true}`), nil }

// Connection is one mail service's bucket. The secret lives only in the
// secrets store, under a slot derived from ServiceID, never in this value.
type Connection struct {
	ServiceID   string `json:"service_id"`
	Endpoint    string `json:"endpoint,omitempty"`
	Region      string `json:"region,omitempty"`
	Bucket      string `json:"bucket"`
	AccessKeyID string `json:"access_key_id"`
	PathStyle   bool   `json:"path_style"`

	secrets SecretStore
}

// String never includes the secret or the key id.
func (c Connection) String() string {
	return fmt.Sprintf("{ServiceID:%s Bucket:%s Secret:[sealed]}", c.ServiceID, c.Bucket)
}

// GoString covers %#v with the same redaction.
func (c Connection) GoString() string { return "mailstorage.Connection" + c.String() }

// LogValue keeps slog output free of credentials.
func (c Connection) LogValue() slog.Value {
	return slog.GroupValue(slog.String("service_id", c.ServiceID), slog.String("bucket", c.Bucket), slog.String("secret", "[sealed]"))
}

// SecretSlot is the secrets-store service name for one mail service.
func SecretSlot(serviceID string) string { return "mail-storage/" + serviceID }

// ValidServiceID reports whether id is safe to use as a key prefix.
func ValidServiceID(id string) bool { return serviceIDPattern.MatchString(id) }

// New validates spec, seals its secret and returns the connection.
func New(ctx context.Context, store SecretStore, spec Spec) (Connection, error) {
	if store == nil {
		return Connection{}, errors.New("mailstorage: secret store is required")
	}
	if !ValidServiceID(spec.ServiceID) {
		return Connection{}, errors.New("mailstorage: invalid service id")
	}
	if strings.TrimSpace(spec.Bucket) == "" || spec.AccessKeyID == "" || spec.Secret == "" {
		return Connection{}, errors.New("mailstorage: bucket, access key id and secret are required")
	}
	if spec.Endpoint != "" {
		if err := objectstore.ValidateEndpoint(spec.Endpoint); err != nil {
			return Connection{}, fmt.Errorf("mailstorage: %w", err)
		}
	}
	if err := store.SetValue(ctx, SecretSlot(spec.ServiceID), secretEnvKey, spec.Secret); err != nil {
		return Connection{}, fmt.Errorf("mailstorage: seal secret: %w", err)
	}
	return Load(store, Connection{
		ServiceID: spec.ServiceID, Endpoint: strings.TrimRight(spec.Endpoint, "/"), Region: spec.Region,
		Bucket: spec.Bucket, AccessKeyID: spec.AccessKeyID, PathStyle: spec.PathStyle,
	}), nil
}

// Load rebinds a stored record to the secrets store.
func Load(store SecretStore, rec Connection) Connection {
	rec.secrets = store
	return rec
}

// Forget removes the sealed secret, for service deletion.
func (c Connection) Forget(ctx context.Context) error {
	if c.secrets == nil {
		return errors.New("mailstorage: connection is not bound to a secret store")
	}
	return c.secrets.DeleteValue(ctx, SecretSlot(c.ServiceID), secretEnvKey)
}

func (c Connection) config(ctx context.Context) (objectstore.Config, error) {
	if c.secrets == nil || !ValidServiceID(c.ServiceID) {
		return objectstore.Config{}, errors.New("mailstorage: connection is not usable")
	}
	secret, err := c.secrets.Resolve(ctx, SecretSlot(c.ServiceID), secretEnvKey)
	if err != nil {
		return objectstore.Config{}, fmt.Errorf("mailstorage: open secret: %w", err)
	}
	return objectstore.Config{
		Endpoint: c.Endpoint, Region: c.Region, Bucket: c.Bucket,
		AccessKeyID: c.AccessKeyID, SecretAccessKey: secret, PathStyle: c.PathStyle,
		HTTPClient: netguard.NewClientAllowing(AllowPrivateEnv),
	}, nil
}
