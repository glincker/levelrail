package proxyroutes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Where the official nginx image reads configuration, and where certbot
// keeps certificates and its webroot by convention.
const (
	nginxConfDir      = "/etc/nginx/conf.d"
	nginxSitesDir     = "/etc/nginx/sites-enabled"
	nginxMainConf     = "/etc/nginx/nginx.conf"
	nginxLetsEncrypt  = "/etc/letsencrypt"
	nginxCertbotLive  = "/etc/letsencrypt/live"
	nginxCertbotWebrt = "/var/www/certbot"
	nginxFileExt      = ".conf"
)

var (
	nginxPathRE    = regexp.MustCompile(`^/[A-Za-z0-9._/-]*$`)
	nginxIncludeRE = regexp.MustCompile(`(?m)^\s*include\s+([^;]+);`)
)

type nginxProvider struct{}

func (nginxProvider) Kind() string { return KindNginx }
func (nginxProvider) Mode() string { return store.ProxyIntegrationNginxFile }
func (nginxProvider) Ext() string  { return nginxFileExt }

func (nginxProvider) Detect(c Container) Detection {
	d := Detection{Kind: c.Kind, Container: c.Name, Image: c.Image, HostNetwork: c.NetworkMode == "host"}
	for _, m := range c.Mappings {
		d.PublishedPorts = append(d.PublishedPorts, m.Public)
	}
	d.PublishedPorts = uniqueInts(d.PublishedPorts)
	d.detectUpstream(c)
	d.detectNginxPorts(c)
	d.detectNginxDir(c)
	d.detectNginxCerts(c)
	d.finish()
	return d
}

func (d *Detection) detectNginxPorts(c Container) {
	if pub, ok := d.publicPort(c, 80); ok {
		d.PublicHTTPPort = pub
	} else {
		d.Missing = append(d.Missing, "container port 80 is not published on the host")
	}
	if pub, ok := d.publicPort(c, 443); ok {
		d.PublicHTTPSPort = pub
	} else {
		d.Missing = append(d.Missing, "container port 443 is not published on the host")
	}
}

func (d *Detection) detectNginxDir(c Container) {
	for _, dir := range []string{nginxConfDir, nginxSitesDir} {
		host, ok := hostPath(c.Mounts, dir)
		if !ok {
			continue
		}
		d.ContainerDir, d.DynamicDir = dir, host
		if len(c.NginxConf) > 0 && !nginxIncludes(c.NginxConf, dir) {
			d.Missing = append(d.Missing, "nginx.conf does not include "+dir+", so files written there would not be loaded")
		}
		return
	}
	d.dirMissing = append(d.dirMissing, "neither "+nginxConfDir+" nor "+nginxSitesDir+" is on a host mount, so this host cannot write into it; pass dynamic_dir")
}

// nginxIncludes reports whether conf has an include directive that reads
// files from dir.
func nginxIncludes(conf []byte, dir string) bool {
	base := dir[strings.LastIndex(dir, "/")+1:]
	for _, m := range nginxIncludeRE.FindAllSubmatch(conf, -1) {
		if strings.Contains(string(m[1]), base+"/") {
			return true
		}
	}
	return false
}

func (d *Detection) detectNginxCerts(c Container) {
	if _, ok := hostPath(c.Mounts, nginxCertbotLive); ok {
		d.CertDir = nginxCertbotLive
	} else {
		d.certMissing = append(d.certMissing, "no certificate directory: nothing is mounted at "+nginxLetsEncrypt+" (certbot's layout); mount it or pass cert_dir")
	}
	if _, ok := hostPath(c.Mounts, nginxCertbotWebrt); ok {
		d.Webroot = nginxCertbotWebrt
	}
}

// NginxConfPath is the host path of nginx.conf when it is on a mount.
func NginxConfPath(c Container) (string, bool) { return hostPath(c.Mounts, nginxMainConf) }

// OverrideCertDir uses dir, given by the operator, as the in-container
// certificate directory and drops the gaps it resolves.
func (d *Detection) OverrideCertDir(dir string) {
	d.CertDir = dir
	d.Missing = withoutAll(d.Missing, d.certMissing)
	d.certMissing = nil
	d.Complete = len(d.Missing) == 0
}

func withoutAll(all, drop []string) []string {
	skip := map[string]bool{}
	for _, m := range drop {
		skip[m] = true
	}
	kept := []string{}
	for _, m := range all {
		if !skip[m] {
			kept = append(kept, m)
		}
	}
	return kept
}

// ValidateNginxPath checks a certificate directory or webroot before it is
// written into a configuration file.
func ValidateNginxPath(kind, p string) error {
	if !nginxPathRE.MatchString(p) || strings.Contains(p, "..") {
		return fmt.Errorf("%s %q must be an absolute path of letters, digits, '.', '_', '-' and '/'", kind, p)
	}
	return nil
}

func (nginxProvider) Render(ns Namespace, r Route) ([]byte, error) {
	d, err := ValidateDomain(r.Domain)
	if err != nil {
		return nil, err
	}
	if d != r.Domain {
		return nil, fmt.Errorf("domain %q must be lowercase", r.Domain)
	}
	if err := ValidateUpstream(r.Upstream); err != nil {
		return nil, err
	}
	if r.CertDir == "" {
		return nil, errors.New("nginx routes need a certificate directory (cert_dir)")
	}
	if err := ValidateNginxPath("certificate directory", r.CertDir); err != nil {
		return nil, err
	}
	if r.Webroot != "" {
		if err := ValidateNginxPath("webroot", r.Webroot); err != nil {
			return nil, err
		}
	}
	certDir := strings.TrimSuffix(r.CertDir, "/")
	conn := "$" + string(ns) + "_connection"
	var b bytes.Buffer
	fmt.Fprintf(&b, "%s\n# Do not edit: rewritten on every reconcile. Remove the domain instead.\n", ns.Header())
	fmt.Fprintf(&b, "server {\n\tlisten 80;\n\tlisten [::]:80;\n\tserver_name %s;\n", d)
	if r.Webroot != "" {
		fmt.Fprintf(&b, "\tlocation /.well-known/acme-challenge/ {\n\t\troot %s;\n\t}\n", r.Webroot)
	}
	b.WriteString("\tlocation / {\n\t\treturn 301 https://$host$request_uri;\n\t}\n}\n")
	fmt.Fprintf(&b, "server {\n\tlisten 443 ssl;\n\tlisten [::]:443 ssl;\n\tserver_name %s;\n", d)
	fmt.Fprintf(&b, "\tssl_certificate %s/%s/fullchain.pem;\n\tssl_certificate_key %s/%s/privkey.pem;\n", certDir, d, certDir, d)
	fmt.Fprintf(&b, "\tlocation / {\n\t\tset %s upgrade;\n\t\tif ($http_upgrade = \"\") {\n\t\t\tset %s \"\";\n\t\t}\n", conn, conn)
	fmt.Fprintf(&b, "\t\tproxy_pass http://%s;\n\t\tproxy_http_version 1.1;\n", r.Upstream)
	b.WriteString("\t\tproxy_set_header Host $host;\n\t\tproxy_set_header X-Real-IP $remote_addr;\n")
	b.WriteString("\t\tproxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n\t\tproxy_set_header X-Forwarded-Proto $scheme;\n\t\tproxy_set_header X-Forwarded-Host $host;\n")
	fmt.Fprintf(&b, "\t\tproxy_set_header Upgrade $http_upgrade;\n\t\tproxy_set_header Connection %s;\n\t}\n}\n", conn)
	return b.Bytes(), nil
}

func (nginxProvider) Validate(ctx context.Context, ex Exec, container string) error {
	if _, err := ex.Exec(ctx, container, []string{"nginx", "-t"}); err != nil {
		return fmt.Errorf("nginx -t rejected the configuration: %w", err)
	}
	return nil
}

func (nginxProvider) Reload(ctx context.Context, ex Exec, container string) error {
	if _, err := ex.Exec(ctx, container, []string{"nginx", "-s", "reload"}); err != nil {
		return fmt.Errorf("nginx reload failed: %w", err)
	}
	return nil
}
