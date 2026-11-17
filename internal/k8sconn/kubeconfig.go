package k8sconn

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

const defaultKubeconfigMaxBytes = 64 << 10

// ErrRefused marks a kubeconfig field Levelrail will not honour.
var ErrRefused = errors.New("k8sconn: kubeconfig field refused")

// Credentials is one kubeconfig context reduced to inline material.
type Credentials struct {
	Server     string
	ServerName string
	CA         []byte
	Token      string
	ClientCert []byte
	ClientKey  []byte
}

type rawConfig struct {
	CurrentContext string `yaml:"current-context"`
	Clusters       []struct {
		Name    string     `yaml:"name"`
		Cluster rawCluster `yaml:"cluster"`
	} `yaml:"clusters"`
	Users []struct {
		Name string  `yaml:"name"`
		User rawUser `yaml:"user"`
	} `yaml:"users"`
	Contexts []struct {
		Name    string `yaml:"name"`
		Context struct {
			Cluster string `yaml:"cluster"`
			User    string `yaml:"user"`
		} `yaml:"context"`
	} `yaml:"contexts"`
}

type rawCluster struct {
	Server                string `yaml:"server"`
	CAData                string `yaml:"certificate-authority-data"`
	CAFile                string `yaml:"certificate-authority"`
	InsecureSkipTLSVerify bool   `yaml:"insecure-skip-tls-verify"`
	ProxyURL              string `yaml:"proxy-url"`
	TLSServerName         string `yaml:"tls-server-name"`
}

type rawUser struct {
	Token          string         `yaml:"token"`
	TokenFile      string         `yaml:"tokenFile"`
	ClientCertData string         `yaml:"client-certificate-data"`
	ClientKeyData  string         `yaml:"client-key-data"`
	ClientCertFile string         `yaml:"client-certificate"`
	ClientKeyFile  string         `yaml:"client-key"`
	Exec           map[string]any `yaml:"exec"`
	AuthProvider   map[string]any `yaml:"auth-provider"`
	Username       string         `yaml:"username"`
	Password       string         `yaml:"password"`
	Impersonate    string         `yaml:"as"`
}

// ParseKubeconfig reduces one context (empty means current-context) to inline
// credentials. Only the chosen context is validated, so merged files that carry
// exec users for other clusters still import.
func ParseKubeconfig(data []byte, contextName string, maxBytes int) (Credentials, error) {
	if maxBytes <= 0 {
		maxBytes = defaultKubeconfigMaxBytes
	}
	if len(data) == 0 || len(data) > maxBytes {
		return Credentials{}, errors.New("k8sconn: kubeconfig is empty or over the size cap")
	}
	var cfg rawConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Credentials{}, errors.New("k8sconn: kubeconfig is not valid yaml")
	}
	if contextName == "" {
		contextName = cfg.CurrentContext
	}
	var clusterName, userName string
	found := false
	for _, c := range cfg.Contexts {
		if c.Name == contextName {
			clusterName, userName, found = c.Context.Cluster, c.Context.User, true
		}
	}
	if !found {
		return Credentials{}, fmt.Errorf("k8sconn: context %q not found", contextName)
	}
	var cl *rawCluster
	for i := range cfg.Clusters {
		if cfg.Clusters[i].Name == clusterName {
			cl = &cfg.Clusters[i].Cluster
		}
	}
	var us *rawUser
	for i := range cfg.Users {
		if cfg.Users[i].Name == userName {
			us = &cfg.Users[i].User
		}
	}
	if cl == nil || us == nil {
		return Credentials{}, errors.New("k8sconn: context references a missing cluster or user")
	}
	if err := refuse(cl, us); err != nil {
		return Credentials{}, err
	}
	return build(cl, us)
}

func refuse(cl *rawCluster, us *rawUser) error {
	checks := []struct {
		bad  bool
		name string
	}{
		{cl.CAFile != "", "certificate-authority (file path)"},
		{cl.InsecureSkipTLSVerify, "insecure-skip-tls-verify"},
		{cl.ProxyURL != "", "proxy-url"},
		{us.TokenFile != "", "tokenFile (file path)"},
		{us.ClientCertFile != "", "client-certificate (file path)"},
		{us.ClientKeyFile != "", "client-key (file path)"},
		{us.Exec != nil, "exec"},
		{us.AuthProvider != nil, "auth-provider"},
		{us.Username != "" || us.Password != "", "basic auth"},
		{us.Impersonate != "", "impersonation"},
	}
	for _, c := range checks {
		if c.bad {
			return fmt.Errorf("%w: %s", ErrRefused, c.name)
		}
	}
	return nil
}

func build(cl *rawCluster, us *rawUser) (Credentials, error) {
	out := Credentials{Server: cl.Server, ServerName: cl.TLSServerName, Token: us.Token}
	if out.Server == "" {
		return Credentials{}, errors.New("k8sconn: cluster has no server")
	}
	if _, err := ParseServer(out.Server); err != nil {
		return Credentials{}, err
	}
	var err error
	if cl.CAData != "" {
		if out.CA, err = decodePEM(cl.CAData); err != nil {
			return Credentials{}, errors.New("k8sconn: certificate-authority-data is not valid base64")
		}
		if !x509.NewCertPool().AppendCertsFromPEM(out.CA) {
			return Credentials{}, errors.New("k8sconn: certificate-authority-data holds no certificate")
		}
	}
	if us.ClientCertData != "" || us.ClientKeyData != "" {
		c, e1 := decodePEM(us.ClientCertData)
		k, e2 := decodePEM(us.ClientKeyData)
		if e1 != nil || e2 != nil {
			return Credentials{}, errors.New("k8sconn: client certificate or key is not valid base64")
		}
		if _, err := tls.X509KeyPair(c, k); err != nil {
			return Credentials{}, errors.New("k8sconn: client certificate and key do not match")
		}
		out.ClientCert, out.ClientKey = c, k
	}
	if out.Token == "" && out.ClientCert == nil {
		return Credentials{}, errors.New("k8sconn: context has neither a token nor a client certificate")
	}
	return out, nil
}

func decodePEM(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
