// Package secrets reads and writes SSM Parameter Store and Secrets Manager.
// Values travel as awsx.Secret and are scrubbed from every error.
package secrets

import (
	"context"
	"errors"
	"strings"

	"github.com/glincker/levelrail/internal/awsx"
)

// SSM and Secrets Manager services (target prefixes checked against the botocore models).
var (
	SSM            = awsx.Service{Name: "ssm", Prefix: "ssm", SigningName: "ssm", TargetPrefix: "AmazonSSM"}
	SecretsManager = awsx.Service{Name: "secretsmanager", Prefix: "secretsmanager", SigningName: "secretsmanager", TargetPrefix: "secretsmanager"}
)

// Redacted is what Redact writes in place of a value.
const Redacted = "[redacted]"

// Redact replaces every non-empty value in text, so logs and errors cannot carry one.
func Redact(text string, values ...string) string {
	for _, v := range values {
		if v != "" {
			text = strings.ReplaceAll(text, v, Redacted)
		}
	}
	return text
}

type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

// scrub keeps the awsx error chain for class checks but rewrites its text.
func scrub(err error, values ...string) error {
	if err == nil {
		return nil
	}
	return &redactedError{msg: Redact(err.Error(), values...), err: err}
}

// Client is the lazy secrets module.
type Client struct{ aws *awsx.Client }

// New wraps a shared client.
func New(c *awsx.Client) *Client { return &Client{aws: c} }

// Parameter is one SSM parameter. Value is a Secret.
type Parameter struct {
	Name  string
	Type  string
	Value awsx.Secret
}

// GetParametersByPath reads parameters under a path, decrypting SecureStrings.
func (c *Client) GetParametersByPath(ctx context.Context, region, path string, recursive bool) ([]Parameter, error) {
	var res []Parameter
	token := ""
	for {
		in := map[string]any{"Path": path, "Recursive": recursive, "WithDecryption": true}
		if token != "" {
			in["NextToken"] = token
		}
		var out struct {
			Params []struct {
				Name  string      `json:"Name"`
				Type  string      `json:"Type"`
				Value awsx.Secret `json:"Value"`
			} `json:"Parameters"`
			Next string `json:"NextToken"`
		}
		if err := c.aws.JSON(ctx, SSM, region, "GetParametersByPath", in, &out); err != nil {
			return nil, err
		}
		for _, p := range out.Params {
			res = append(res, Parameter{Name: p.Name, Type: p.Type, Value: p.Value})
		}
		if out.Next == "" {
			return res, nil
		}
		token = out.Next
	}
}

// PutParameter writes a SecureString and returns its new version.
func (c *Client) PutParameter(ctx context.Context, region, name string, value awsx.Secret, overwrite bool) (int64, error) {
	in := map[string]any{"Name": name, "Value": value.Reveal(), "Type": "SecureString", "Overwrite": overwrite}
	var out struct {
		Version int64 `json:"Version"`
	}
	if err := c.aws.JSON(ctx, SSM, region, "PutParameter", in, &out); err != nil {
		return 0, scrub(err, value.Reveal())
	}
	return out.Version, nil
}

// SecretSummary is one Secrets Manager entry without its value.
type SecretSummary struct{ Name, ARN string }

// ListSecrets pages through the secrets in a region.
func (c *Client) ListSecrets(ctx context.Context, region string) ([]SecretSummary, error) {
	var res []SecretSummary
	token := ""
	for {
		in := map[string]any{}
		if token != "" {
			in["NextToken"] = token
		}
		var out struct {
			List []struct {
				Name string `json:"Name"`
				ARN  string `json:"ARN"`
			} `json:"SecretList"`
			Next string `json:"NextToken"`
		}
		if err := c.aws.JSON(ctx, SecretsManager, region, "ListSecrets", in, &out); err != nil {
			return nil, err
		}
		for _, s := range out.List {
			res = append(res, SecretSummary{Name: s.Name, ARN: s.ARN})
		}
		if out.Next == "" {
			return res, nil
		}
		token = out.Next
	}
}

// GetSecretValue returns the string value of a secret.
func (c *Client) GetSecretValue(ctx context.Context, region, id string) (awsx.Secret, error) {
	var out struct {
		Value awsx.Secret `json:"SecretString"`
	}
	if err := c.aws.JSON(ctx, SecretsManager, region, "GetSecretValue", map[string]string{"SecretId": id}, &out); err != nil {
		return "", err
	}
	if out.Value == "" {
		return "", errors.New("secrets: secret has no string value")
	}
	return out.Value, nil
}

// PutSecretValue stores a new version of an existing secret.
func (c *Client) PutSecretValue(ctx context.Context, region, id string, value awsx.Secret) error {
	in := map[string]string{"SecretId": id, "SecretString": value.Reveal()}
	return scrub(c.aws.JSON(ctx, SecretsManager, region, "PutSecretValue", in, nil), value.Reveal())
}
