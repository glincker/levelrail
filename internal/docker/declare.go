package docker

// CreateDeclaration is what Create says a container needs beyond the
// hardened default, so a Docker API guard can tell a declared bind mount
// or GPU request from an injected one.
type CreateDeclaration struct {
	Name        string
	HostNetwork bool
	GPU         bool
	BindPaths   []string
	CapAdd      []string
}

// CreateDeclarer receives a declaration before each container create; the
// returned func withdraws it once the create call returns.
type CreateDeclarer interface {
	DeclareCreate(CreateDeclaration) func()
}

// WithHost makes the Client dial host (unix:///path) instead of the
// detected socket, keeping Runtime() describing the real daemon.
func WithHost(host string) ClientOption {
	return func(c *Client) { c.host = host }
}

// WithCreateDeclarer reports every Create to d before it reaches Docker.
func WithCreateDeclarer(d CreateDeclarer) ClientOption {
	return func(c *Client) { c.declarer = d }
}

func (c *Client) declare(spec ContainerSpec) func() {
	if c.declarer == nil {
		return func() {}
	}
	d := CreateDeclaration{
		Name:        spec.Name,
		HostNetwork: spec.NetworkMode == "host",
		GPU:         spec.GPU != nil,
		CapAdd:      append([]string(nil), spec.CapAdd...),
	}
	for _, m := range spec.BindMounts {
		d.BindPaths = append(d.BindPaths, m.HostPath)
	}
	return c.declarer.DeclareCreate(d)
}
