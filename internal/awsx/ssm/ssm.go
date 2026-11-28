// Package ssm reads managed instance status and runs allowlisted commands over signed JSON 1.1.
package ssm

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/awsx"
)

// Service is the Systems Manager API.
var Service = awsx.Service{Name: "ssm", Prefix: "ssm", SigningName: "ssm", TargetPrefix: "AmazonSSM"}

// ErrRefused marks a command or instance id the policy rejects.
var ErrRefused = errors.New("ssm: refused")

var instanceRe = regexp.MustCompile(`^m?i-[0-9a-f]{8,17}$`)
var commandIDRe = regexp.MustCompile(`^[0-9a-f-]{36}$`)

// Client is the lazy SSM module.
type Client struct{ aws *awsx.Client }

// New wraps a shared client.
func New(c *awsx.Client) *Client { return &Client{aws: c} }

// ValidInstance reports whether id looks like an EC2 or hybrid managed instance id.
func ValidInstance(id string) bool { return instanceRe.MatchString(id) }

// Instance is one managed instance row.
type Instance struct {
	ID, PingStatus, Platform, PlatformName, AgentVersion, ComputerName string
	LastPing                                                           time.Time
}

// DescribeInstances lists managed instances, or only one when id is set.
func (c *Client) DescribeInstances(ctx context.Context, region, id string) ([]Instance, error) {
	var res []Instance
	token := ""
	for {
		in := map[string]any{}
		if id != "" {
			in["Filters"] = []map[string]any{{"Key": "InstanceIds", "Values": []string{id}}}
		}
		if token != "" {
			in["NextToken"] = token
		}
		var out struct {
			List []struct {
				ID           string         `json:"InstanceId"`
				Ping         string         `json:"PingStatus"`
				Platform     string         `json:"PlatformType"`
				PlatformName string         `json:"PlatformName"`
				Agent        string         `json:"AgentVersion"`
				Computer     string         `json:"ComputerName"`
				LastPing     awsx.EpochTime `json:"LastPingDateTime"`
			} `json:"InstanceInformationList"`
			Next string `json:"NextToken"`
		}
		if err := c.aws.JSON(ctx, Service, region, "DescribeInstanceInformation", in, &out); err != nil {
			return nil, err
		}
		for _, i := range out.List {
			res = append(res, Instance{ID: i.ID, PingStatus: i.Ping, Platform: i.Platform, PlatformName: i.PlatformName, AgentVersion: i.Agent, ComputerName: i.Computer, LastPing: i.LastPing.Time})
		}
		if out.Next == "" {
			return res, nil
		}
		token = out.Next
	}
}

// Allowed reports whether command matches an allowlist entry exactly. An empty list refuses all.
func Allowed(allow []string, command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	for _, a := range allow {
		if strings.TrimSpace(a) == command {
			return true
		}
	}
	return false
}

// SendCommand runs one allowlisted shell command on a single instance and returns the command id.
func (c *Client) SendCommand(ctx context.Context, region, instanceID, command string, allow []string, timeoutSeconds int) (string, error) {
	if !ValidInstance(instanceID) {
		return "", ErrRefused
	}
	if !Allowed(allow, command) {
		return "", ErrRefused
	}
	in := map[string]any{
		"InstanceIds":    []string{instanceID},
		"DocumentName":   "AWS-RunShellScript",
		"Parameters":     map[string][]string{"commands": {strings.TrimSpace(command)}},
		"TimeoutSeconds": timeoutSeconds,
		"Comment":        "levelrail",
	}
	var out struct {
		Command struct {
			ID string `json:"CommandId"`
		} `json:"Command"`
	}
	if err := c.aws.JSON(ctx, Service, region, "SendCommand", in, &out); err != nil {
		return "", err
	}
	return out.Command.ID, nil
}

// Invocation is the result of one command on one instance.
type Invocation struct {
	Status, Detail, Stdout, Stderr string
	ExitCode                       int
}

// GetInvocation reads the status and output of a command on an instance.
func (c *Client) GetInvocation(ctx context.Context, region, commandID, instanceID string) (Invocation, error) {
	if !ValidInstance(instanceID) || !commandIDRe.MatchString(commandID) {
		return Invocation{}, ErrRefused
	}
	var out struct {
		Status string `json:"Status"`
		Detail string `json:"StatusDetails"`
		Code   int    `json:"ResponseCode"`
		Stdout string `json:"StandardOutputContent"`
		Stderr string `json:"StandardErrorContent"`
	}
	if err := c.aws.JSON(ctx, Service, region, "GetCommandInvocation", map[string]string{"CommandId": commandID, "InstanceId": instanceID}, &out); err != nil {
		return Invocation{}, err
	}
	return Invocation{Status: out.Status, Detail: out.Detail, ExitCode: out.Code, Stdout: out.Stdout, Stderr: out.Stderr}, nil
}

// Session is the token info a client needs to open a session with the session manager plugin.
// The token is a secret, so callers must not log it.
type Session struct {
	ID, Token, StreamURL string
}

// StartSession opens a session and returns its token info. No stream is proxied here.
func (c *Client) StartSession(ctx context.Context, region, instanceID string) (Session, error) {
	if !ValidInstance(instanceID) {
		return Session{}, ErrRefused
	}
	var out struct {
		ID     string `json:"SessionId"`
		Token  string `json:"TokenValue"`
		Stream string `json:"StreamUrl"`
	}
	if err := c.aws.JSON(ctx, Service, region, "StartSession", map[string]string{"Target": instanceID}, &out); err != nil {
		return Session{}, err
	}
	return Session{ID: out.ID, Token: out.Token, StreamURL: out.Stream}, nil
}
