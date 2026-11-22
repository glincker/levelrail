// Package ecr talks to Elastic Container Registry over signed JSON 1.1.
package ecr

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/awsx"
	"github.com/GLINCKER/levelrail/internal/cloudguard"
)

// Service is the ECR API (target prefix and endpoint checked against the botocore model).
var Service = awsx.Service{Name: "ecr", Prefix: "api.ecr", SigningName: "ecr", TargetPrefix: "AmazonEC2ContainerRegistry_V20150921"}

// ServiceName is the key used in the guard ServicesEnabled map.
const ServiceName = "ecr"

// ErrNotEmpty is returned when DeleteRepository is asked to remove images.
var ErrNotEmpty = errors.New("ecr: repository is not empty")

// ErrRefused wraps a guard refusal.
var ErrRefused = errors.New("ecr: create refused")

// Guard is the create gate and tag source.
type Guard interface {
	Check(ctx context.Context, r cloudguard.CreateRequest) cloudguard.Decision
	Tags(m cloudguard.TagMeta) map[string]string
}

// Options sets the default lifecycle policy. Zero values skip that rule.
type Options struct {
	UntaggedExpireDays int
	KeepTaggedImages   int
}

// Client is the lazy ECR module; it makes no call until a method runs.
type Client struct {
	aws   *awsx.Client
	guard Guard
	opts  Options
}

// New wraps a shared client and guard.
func New(c *awsx.Client, g Guard, o Options) *Client { return &Client{aws: c, guard: g, opts: o} }

// AuthToken is the data for a docker login. Password is a Secret.
type AuthToken struct {
	Registry  string
	User      string
	Password  awsx.Secret
	ExpiresAt time.Time
}

// GetAuthorizationToken returns the registry login for the caller's account.
func (c *Client) GetAuthorizationToken(ctx context.Context, region string) (AuthToken, error) {
	var out struct {
		Data []struct {
			Token    awsx.Secret    `json:"authorizationToken"`
			Expires  awsx.EpochTime `json:"expiresAt"`
			Endpoint string         `json:"proxyEndpoint"`
		} `json:"authorizationData"`
	}
	if err := c.aws.JSON(ctx, Service, region, "GetAuthorizationToken", struct{}{}, &out); err != nil {
		return AuthToken{}, err
	}
	if len(out.Data) == 0 {
		return AuthToken{}, errors.New("ecr: no authorization data returned")
	}
	d := out.Data[0]
	raw, err := base64.StdEncoding.DecodeString(d.Token.Reveal())
	if err != nil {
		return AuthToken{}, errors.New("ecr: authorization token is not valid base64")
	}
	user, pass, ok := strings.Cut(string(raw), ":")
	if !ok {
		return AuthToken{}, errors.New("ecr: authorization token has no user")
	}
	return AuthToken{
		Registry: strings.TrimPrefix(d.Endpoint, "https://"), User: user,
		Password: awsx.Secret(pass), ExpiresAt: d.Expires.Time,
	}, nil
}

// Repository is the read view of one repository.
type Repository struct {
	Name       string
	URI        string
	ARN        string
	ScanOnPush bool
	CreatedAt  time.Time
}

// DescribeRepositories lists repositories, all of them or the named ones.
func (c *Client) DescribeRepositories(ctx context.Context, region string, names ...string) ([]Repository, error) {
	var res []Repository
	token := ""
	for {
		in := map[string]any{}
		if len(names) > 0 {
			in["repositoryNames"] = names
		}
		if token != "" {
			in["nextToken"] = token
		}
		var out struct {
			Repos []struct {
				Name    string         `json:"repositoryName"`
				URI     string         `json:"repositoryUri"`
				ARN     string         `json:"repositoryArn"`
				Created awsx.EpochTime `json:"createdAt"`
				Scan    struct {
					OnPush bool `json:"scanOnPush"`
				} `json:"imageScanningConfiguration"`
			} `json:"repositories"`
			Next string `json:"nextToken"`
		}
		if err := c.aws.JSON(ctx, Service, region, "DescribeRepositories", in, &out); err != nil {
			return nil, err
		}
		for _, r := range out.Repos {
			res = append(res, Repository{Name: r.Name, URI: r.URI, ARN: r.ARN, ScanOnPush: r.Scan.OnPush, CreatedAt: r.Created.Time})
		}
		if out.Next == "" {
			return res, nil
		}
		token = out.Next
	}
}

// CreateInput describes one repository create.
type CreateInput struct {
	Account, Region, Name string
	Meta                  cloudguard.TagMeta
	EstimatedMonthlyUSD   float64
}

// CreateRepository goes through the guard, turns scan on push on, and puts the
// default lifecycle policy. A failed policy call leaves the repository created
// and returns the error so the caller can retry PutLifecyclePolicy.
func (c *Client) CreateRepository(ctx context.Context, in CreateInput) (Repository, error) {
	tags := c.guard.Tags(in.Meta)
	d := c.guard.Check(ctx, cloudguard.CreateRequest{
		Account: in.Account, Service: ServiceName, Resource: in.Name,
		EstimatedMonthlyUSD: in.EstimatedMonthlyUSD, Tags: tags,
	})
	if !d.Allowed {
		return Repository{}, fmt.Errorf("%w: %s", ErrRefused, d.Message)
	}
	req := map[string]any{
		"repositoryName":             in.Name,
		"imageScanningConfiguration": map[string]bool{"scanOnPush": true},
		"tags":                       awsx.TagList(tags),
	}
	var out struct {
		Repo struct {
			Name string `json:"repositoryName"`
			URI  string `json:"repositoryUri"`
			ARN  string `json:"repositoryArn"`
		} `json:"repository"`
	}
	if err := c.aws.JSON(ctx, Service, in.Region, "CreateRepository", req, &out); err != nil {
		return Repository{}, err
	}
	repo := Repository{Name: out.Repo.Name, URI: out.Repo.URI, ARN: out.Repo.ARN, ScanOnPush: true}
	if text := c.lifecycleText(); text != "" {
		if err := c.PutLifecyclePolicy(ctx, in.Region, in.Name, text); err != nil {
			return repo, err
		}
	}
	return repo, nil
}

// PutLifecyclePolicy sets the policy JSON text on a repository.
func (c *Client) PutLifecyclePolicy(ctx context.Context, region, name, text string) error {
	return c.aws.JSON(ctx, Service, region, "PutLifecyclePolicy",
		map[string]string{"repositoryName": name, "lifecyclePolicyText": text}, nil)
}

type lifecycleRule struct {
	Priority    int               `json:"rulePriority"`
	Description string            `json:"description"`
	Selection   map[string]any    `json:"selection"`
	Action      map[string]string `json:"action"`
}

// lifecycleText builds the default policy. The rule with tagStatus any must
// have the highest priority number, per the ECR lifecycle policy docs.
func (c *Client) lifecycleText() string {
	var rules []lifecycleRule
	if c.opts.UntaggedExpireDays > 0 {
		rules = append(rules, lifecycleRule{
			Priority: len(rules) + 1, Description: "expire untagged images",
			Selection: map[string]any{"tagStatus": "untagged", "countType": "sinceImagePushed", "countUnit": "days", "countNumber": c.opts.UntaggedExpireDays},
			Action:    map[string]string{"type": "expire"},
		})
	}
	if c.opts.KeepTaggedImages > 0 {
		rules = append(rules, lifecycleRule{
			Priority: len(rules) + 1, Description: "keep the newest images",
			Selection: map[string]any{"tagStatus": "any", "countType": "imageCountMoreThan", "countNumber": c.opts.KeepTaggedImages},
			Action:    map[string]string{"type": "expire"},
		})
	}
	if len(rules) == 0 {
		return ""
	}
	b, _ := json.Marshal(map[string]any{"rules": rules})
	return string(b)
}

// DeleteRepository removes a repository only when it holds no images.
func (c *Client) DeleteRepository(ctx context.Context, region, name string) error {
	var out struct {
		IDs []json.RawMessage `json:"imageIds"`
	}
	if err := c.aws.JSON(ctx, Service, region, "ListImages", map[string]any{"repositoryName": name, "maxResults": 1}, &out); err != nil {
		return err
	}
	if len(out.IDs) > 0 {
		return ErrNotEmpty
	}
	return c.aws.JSON(ctx, Service, region, "DeleteRepository", map[string]any{"repositoryName": name, "force": false}, nil)
}
