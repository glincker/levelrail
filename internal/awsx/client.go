// Package awsx is a small SigV4 client for AWS JSON 1.1 and Query APIs. It
// adds no SDK service modules and does nothing until a call is made.
package awsx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// Credentials uses the same keys as the stored AWS cloud account.
type Credentials struct {
	AccessKeyID, SecretAccessKey, SessionToken string
}

// FromMap reads access_key_id, secret_access_key and session_token.
func FromMap(m map[string]string) (Credentials, error) {
	c := Credentials{m["access_key_id"], m["secret_access_key"], m["session_token"]}
	if c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return Credentials{}, errors.New("awsx: credentials are incomplete")
	}
	return c, nil
}

// Doer is the HTTP seam; *http.Client satisfies it.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Options configures a Client. Zero fields take the DefaultOptions values.
type Options struct {
	Credentials Credentials
	HTTP        Doer
	Now         func() time.Time
	Sleep       func(ctx context.Context, d time.Duration) error
	// BaseURL replaces the regional endpoint, for fake servers.
	BaseURL          string
	MaxAttempts      int
	BaseDelay        time.Duration
	MaxResponseBytes int64
}

func envInt(name string, def int64) int64 {
	if v, err := strconv.ParseInt(os.Getenv(name), 10, 64); err == nil && v > 0 {
		return v
	}
	return def
}

// DefaultOptions seeds limits from APP_AWSX_MAX_ATTEMPTS, APP_AWSX_BASE_DELAY_MS
// and APP_AWSX_MAX_RESPONSE_BYTES.
func DefaultOptions(c Credentials) Options {
	return Options{
		Credentials:      c,
		MaxAttempts:      int(envInt("APP_AWSX_MAX_ATTEMPTS", 3)),
		BaseDelay:        time.Duration(envInt("APP_AWSX_BASE_DELAY_MS", 200)) * time.Millisecond,
		MaxResponseBytes: envInt("APP_AWSX_MAX_RESPONSE_BYTES", 8<<20),
	}
}

// Client sends signed requests.
type Client struct {
	o      Options
	creds  aws.Credentials
	signer *v4.Signer
}

// New builds a client. HTTP, Now and Sleep default to real ones.
func New(o Options) (*Client, error) {
	if o.Credentials.AccessKeyID == "" || o.Credentials.SecretAccessKey == "" {
		return nil, errors.New("awsx: credentials are incomplete")
	}
	d := DefaultOptions(o.Credentials)
	if o.MaxAttempts < 1 {
		o.MaxAttempts = d.MaxAttempts
	}
	if o.BaseDelay <= 0 {
		o.BaseDelay = d.BaseDelay
	}
	if o.MaxResponseBytes <= 0 {
		o.MaxResponseBytes = d.MaxResponseBytes
	}
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Sleep == nil {
		o.Sleep = sleepCtx
	}
	return &Client{
		o: o, signer: v4.NewSigner(),
		creds: aws.Credentials{
			AccessKeyID: o.Credentials.AccessKeyID, SecretAccessKey: o.Credentials.SecretAccessKey,
			SessionToken: o.Credentials.SessionToken,
		},
	}, nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Service names one AWS API. TargetPrefix is empty for Query services.
type Service struct {
	Name         string
	Prefix       string
	SigningName  string
	TargetPrefix string
}

// Endpoint resolves the regional endpoint host for a service.
func (s Service) Endpoint(region string) string {
	suffix := "amazonaws.com"
	if strings.HasPrefix(region, "cn-") {
		suffix = "amazonaws.com.cn"
	}
	return "https://" + s.Prefix + "." + region + "." + suffix
}

type attempt struct {
	svc         Service
	region, op  string
	contentType string
	target      string
	body        []byte
	// method, path and endpoint are set by REST calls; JSON and Query keep POST on the service root.
	method, path, endpoint string
}

// JSON sends one JSON 1.1 call and decodes the response into out (may be nil).
func (c *Client) JSON(ctx context.Context, svc Service, region, op string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	a := attempt{svc: svc, region: region, op: op, contentType: "application/x-amz-json-1.1", target: svc.TargetPrefix + "." + op, body: body}
	raw, err := c.do(ctx, a, decodeJSONError)
	if err != nil || out == nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

// Query sends one Query API call (EC2 style) and decodes the XML response.
func (c *Client) Query(ctx context.Context, svc Service, region, action, version string, params url.Values, out any) error {
	v := url.Values{}
	for k, vals := range params {
		v[k] = vals
	}
	v.Set("Action", action)
	v.Set("Version", version)
	a := attempt{svc: svc, region: region, op: action, contentType: "application/x-www-form-urlencoded; charset=utf-8", body: []byte(v.Encode())}
	raw, err := c.do(ctx, a, decodeXMLError)
	if err != nil || out == nil {
		return err
	}
	return xml.Unmarshal(raw, out)
}

func (c *Client) do(ctx context.Context, a attempt, parse func([]byte, http.Header) (string, string)) ([]byte, error) {
	if a.region == "" {
		return nil, errors.New("awsx: region is required")
	}
	var last error
	for n := 0; n < c.o.MaxAttempts; n++ {
		raw, err := c.once(ctx, a, parse)
		if err == nil {
			return raw, nil
		}
		last = err
		var ae *Error
		if !errors.As(err, &ae) || !retryable(ae.Class) || n == c.o.MaxAttempts-1 {
			return nil, err
		}
		if err := c.o.Sleep(ctx, c.o.BaseDelay<<n); err != nil {
			return nil, err
		}
	}
	return nil, last
}

func (c *Client) once(ctx context.Context, a attempt, parse func([]byte, http.Header) (string, string)) ([]byte, error) {
	endpoint := c.o.BaseURL
	if endpoint == "" {
		endpoint = a.endpoint
	}
	if endpoint == "" {
		endpoint = a.svc.Endpoint(a.region)
	}
	method := a.method
	if method == "" {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint+a.path, bytes.NewReader(a.body))
	if err != nil {
		return nil, err
	}
	if a.contentType != "" {
		req.Header.Set("Content-Type", a.contentType)
	}
	if a.target != "" {
		req.Header.Set("X-Amz-Target", a.target)
	}
	sum := sha256.Sum256(a.body)
	sign := a.svc.SigningName
	if sign == "" {
		sign = a.svc.Prefix
	}
	if err := c.signer.SignHTTP(ctx, c.creds, req, hex.EncodeToString(sum[:]), sign, a.region, c.o.Now()); err != nil {
		return nil, errors.New("awsx: sign failed")
	}
	resp, err := c.o.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("awsx: %s %s: transport error", a.svc.Name, a.op)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, c.o.MaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("awsx: %s %s: read failed", a.svc.Name, a.op)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return raw, nil
	}
	code, msg := parse(raw, resp.Header)
	if code == "" {
		code = "HTTP" + strconv.Itoa(resp.StatusCode)
	}
	return nil, &Error{Service: a.svc.Name, Op: a.op, Code: code, Message: msg, Status: resp.StatusCode, Class: classify(code, resp.StatusCode)}
}

// decodeJSONError reads the code from X-Amzn-Errortype or the __type field.
func decodeJSONError(raw []byte, h http.Header) (string, string) {
	var b struct {
		Type    string `json:"__type"`
		Code    string `json:"code"`
		Message string `json:"message"`
		Upper   string `json:"Message"`
	}
	_ = json.Unmarshal(raw, &b)
	code := h.Get("X-Amzn-Errortype")
	if code == "" {
		code = b.Type
	}
	if code == "" {
		code = b.Code
	}
	if i := strings.IndexByte(code, ':'); i >= 0 {
		code = code[:i]
	}
	if i := strings.LastIndexByte(code, '#'); i >= 0 {
		code = code[i+1:]
	}
	msg := b.Message
	if msg == "" {
		msg = b.Upper
	}
	return code, msg
}

func decodeXMLError(raw []byte, _ http.Header) (string, string) {
	var b struct {
		Errors []struct {
			Code    string `xml:"Code"`
			Message string `xml:"Message"`
		} `xml:"Errors>Error"`
		Error struct {
			Code    string `xml:"Code"`
			Message string `xml:"Message"`
		} `xml:"Error"`
	}
	_ = xml.Unmarshal(raw, &b)
	if len(b.Errors) > 0 {
		return b.Errors[0].Code, b.Errors[0].Message
	}
	return b.Error.Code, b.Error.Message
}
