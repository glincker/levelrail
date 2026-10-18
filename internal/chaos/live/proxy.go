package live

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// socketProxy forwards a unix socket to the Docker daemon and can be switched
// off to make the Docker API unreachable without touching the real daemon.
type socketProxy struct {
	path     string
	upstream string
	ln       net.Listener
	mu       sync.Mutex
	down     bool
	conns    map[net.Conn]struct{}
}

func newSocketProxy(dir, daemonHost string) (*socketProxy, error) {
	p := &socketProxy{
		path:     filepath.Join(dir, "docker.sock"),
		upstream: strings.TrimPrefix(daemonHost, "unix://"),
		conns:    map[net.Conn]struct{}{},
	}
	ln, err := net.Listen("unix", p.path)
	if err != nil {
		return nil, err
	}
	p.ln = ln
	go p.serve()
	return p, nil
}

func (p *socketProxy) host() string { return "unix://" + p.path }

func (p *socketProxy) serve() {
	for {
		c, err := p.ln.Accept()
		if err != nil {
			return
		}
		go p.handle(c)
	}
}

func (p *socketProxy) handle(c net.Conn) {
	p.mu.Lock()
	if p.down {
		p.mu.Unlock()
		_ = c.Close()
		return
	}
	up, err := net.Dial("unix", p.upstream)
	if err != nil {
		p.mu.Unlock()
		_ = c.Close()
		return
	}
	p.conns[c], p.conns[up] = struct{}{}, struct{}{}
	p.mu.Unlock()

	done := make(chan struct{}, 2)
	pipe := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		done <- struct{}{}
	}
	go pipe(up, c)
	go pipe(c, up)
	<-done
	_ = c.Close()
	_ = up.Close()
	p.mu.Lock()
	delete(p.conns, c)
	delete(p.conns, up)
	p.mu.Unlock()
}

// setDown drops every open connection and refuses new ones, or accepts again.
func (p *socketProxy) setDown(down bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.down = down
	if !down {
		return
	}
	for c := range p.conns {
		_ = c.Close()
	}
}

func (p *socketProxy) close() {
	_ = p.ln.Close()
	p.setDown(true)
	_ = os.Remove(p.path)
}
