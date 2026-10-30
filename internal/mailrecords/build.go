// Package mailrecords builds, plans, applies and checks the DNS records a
// domain needs for email. It is engine agnostic: the same records serve a
// self-hosted mail server or any external provider, and it costs the server
// nothing beyond the DNS provider calls.
package mailrecords

import (
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"net/netip"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dnszones"
)

// Record kinds, also the values ReplaceExisting accepts.
const (
	KindMX         = "MX"
	KindHost       = "HOST"
	KindSPF        = "SPF"
	KindDKIM       = "DKIM"
	KindDMARC      = "DMARC"
	KindMTASTS     = "MTASTS"
	KindTLSRPT     = "TLSRPT"
	KindAutoconfig = "AUTOCONFIG"
)

var kindOrder = []string{KindHost, KindMX, KindSPF, KindDKIM, KindDMARC, KindMTASTS, KindTLSRPT, KindAutoconfig}

// SPF modes.
const (
	SPFServer  = "server"
	SPFInclude = "include"
)

// Note levels.
const (
	NoteInfo = "info"
	NoteWarn = "warn"
)

// Spec is everything Build needs. Only Domain is required.
type Spec struct {
	Domain        string   `json:"domain"`
	MailHost      string   `json:"mail_host,omitempty"`
	IPv4          string   `json:"ipv4,omitempty"`
	IPv6          string   `json:"ipv6,omitempty"`
	MXPriority    int      `json:"mx_priority,omitempty"`
	Preset        string   `json:"preset,omitempty"`
	SPFMode       string   `json:"spf_mode,omitempty"`
	SPFIncludes   []string `json:"spf_includes,omitempty"`
	DKIMSelector  string   `json:"dkim_selector,omitempty"`
	DKIMPublicKey string   `json:"dkim_public_key,omitempty"`
	DMARCPolicy   string   `json:"dmarc_policy,omitempty"`
	DMARCRua      string   `json:"dmarc_rua,omitempty"`
	DMARCPct      int      `json:"dmarc_pct,omitempty"`
	MTASTSMode    string   `json:"mta_sts_mode,omitempty"`
	MTASTSMaxAge  int      `json:"mta_sts_max_age,omitempty"`
	TLSRPT        bool     `json:"tls_rpt,omitempty"`
	TLSRPTRua     string   `json:"tls_rpt_rua,omitempty"`
	Autoconfig    bool     `json:"autoconfig,omitempty"`
	TTL           int      `json:"ttl,omitempty"`
}

// Note is advice that is not a record: a pending key, a policy file to host.
type Note struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Record is one record set to publish, tagged with the kind that owns it.
type Record struct {
	Kind string             `json:"kind"`
	Set  dnszones.RecordSet `json:"set"`
}

var labelRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ValidHost enforces the package convention: a lower case absolute name with
// at least two labels and no trailing dot, scheme, port or spaces.
func ValidHost(h string) error {
	switch {
	case h != strings.TrimSpace(h) || strings.ContainsAny(h, " /:@\t"):
		return fmt.Errorf("%q is not a bare host name", h)
	case strings.HasSuffix(h, "."):
		return fmt.Errorf("%q has a trailing dot; give the name without it", h)
	case h != strings.ToLower(h):
		return fmt.Errorf("%q must be lower case", h)
	case len(h) > 253:
		return fmt.Errorf("%q is longer than 253 characters", h)
	}
	labels := strings.Split(h, ".")
	if len(labels) < 2 {
		return fmt.Errorf("%q is a relative name; use the full name such as mail.example.com", h)
	}
	for _, l := range labels {
		if !labelRE.MatchString(l) {
			return fmt.Errorf("%q has an invalid label %q", h, l)
		}
	}
	return nil
}

func defaultTTL() int {
	if v, err := strconv.Atoi(os.Getenv("APP_DNS_DEFAULT_TTL")); err == nil && v > 0 {
		return v
	}
	return 300
}

func defaultMTASTSMaxAge() int {
	if v, err := strconv.Atoi(os.Getenv("APP_MAIL_MTASTS_MAX_AGE")); err == nil && v > 0 {
		return v
	}
	return 86400
}

type builder struct {
	domain string
	sets   map[dnszones.Key]*Record
	order  []dnszones.Key
	notes  []Note
}

func (b *builder) rel(fqdn string) string { return dnszones.RelativeName(fqdn, b.domain) }

func (b *builder) add(kind, fqdn, typ, value string) {
	k := dnszones.Key{Name: b.rel(fqdn), Type: typ}
	r, ok := b.sets[k]
	if !ok {
		r = &Record{Kind: kind, Set: dnszones.RecordSet{Name: k.Name, Type: typ}}
		b.sets[k] = r
		b.order = append(b.order, k)
	}
	if !slices.Contains(r.Set.Values, value) {
		r.Set.Values = append(r.Set.Values, value)
	}
}

func (b *builder) note(level, code, msg string) {
	b.notes = append(b.notes, Note{Level: level, Code: code, Message: msg})
}

func (b *builder) inZone(host string) bool {
	return host == b.domain || strings.HasSuffix(host, "."+b.domain)
}

// Build returns the record sets for spec, plus notes for everything that
// cannot be a record. It is pure: no network, no clock.
func Build(spec Spec) ([]Record, []Note, error) {
	domain := spec.Domain
	if err := ValidHost(domain); err != nil {
		return nil, nil, fmt.Errorf("domain: %w", err)
	}
	b := &builder{domain: domain, sets: map[dnszones.Key]*Record{}}
	var preset *Preset
	if spec.Preset != "" {
		p, ok := Presets[strings.ToLower(spec.Preset)]
		if !ok {
			return nil, nil, fmt.Errorf("unknown preset %q, want one of %s", spec.Preset, strings.Join(PresetIDs(), ", "))
		}
		preset = &p
	}
	host, mxHosts, err := b.mailBasics(spec, preset)
	if err != nil {
		return nil, nil, err
	}
	if err := b.spf(spec, preset); err != nil {
		return nil, nil, err
	}
	if err := b.dkim(spec, preset); err != nil {
		return nil, nil, err
	}
	if err := b.dmarc(spec); err != nil {
		return nil, nil, err
	}
	if err := b.transport(spec, host, mxHosts); err != nil {
		return nil, nil, err
	}
	ttl := spec.TTL
	if ttl <= 0 {
		ttl = defaultTTL()
	}
	return b.finish(ttl)
}

func (b *builder) finish(ttl int) ([]Record, []Note, error) {
	out := make([]Record, 0, len(b.order))
	for _, k := range b.order {
		r := *b.sets[k]
		norm, err := dnszones.Normalize(r.Set, b.domain, ttl)
		if err != nil {
			return nil, nil, fmt.Errorf("%s record %s %s: %w", r.Kind, k.Name, k.Type, err)
		}
		for _, v := range norm.Values {
			if r.Set.Type == "TXT" && len(v) > 255 {
				b.note(NoteInfo, "txt_chunked", fmt.Sprintf("%s TXT is %d bytes; it is stored as %d strings of at most 255 bytes, which receivers rejoin.", r.Set.Name, len(v), (len(v)+254)/255))
			}
		}
		r.Set = norm
		out = append(out, r)
	}
	slices.SortStableFunc(out, func(a, c Record) int {
		if d := slices.Index(kindOrder, a.Kind) - slices.Index(kindOrder, c.Kind); d != 0 {
			return d
		}
		return strings.Compare(a.Set.Name+a.Set.Type, c.Set.Name+c.Set.Type)
	})
	return out, b.notes, nil
}

func (b *builder) mailBasics(spec Spec, preset *Preset) (host string, mxHosts []string, err error) {
	if preset != nil {
		for _, m := range preset.MX {
			b.add(KindMX, b.domain, "MX", fmt.Sprintf("%d %s", m.Priority, m.Host))
			mxHosts = append(mxHosts, m.Host)
		}
		b.note(NoteInfo, "preset", fmt.Sprintf("%s values from: %s. %s", preset.Name, preset.Source, preset.Note))
		return "", mxHosts, nil
	}
	host = spec.MailHost
	if host == "" {
		host = "mail." + b.domain
	}
	if err := ValidHost(host); err != nil {
		return "", nil, fmt.Errorf("mail_host: %w", err)
	}
	prio := spec.MXPriority
	if prio == 0 {
		prio = 10
	}
	if prio < 0 || prio > 65535 {
		return "", nil, errors.New("mx_priority must be between 0 and 65535")
	}
	b.add(KindMX, b.domain, "MX", fmt.Sprintf("%d %s", prio, host))
	for _, ip := range []struct{ raw, typ string }{{spec.IPv4, "A"}, {spec.IPv6, "AAAA"}} {
		if ip.raw == "" {
			continue
		}
		addr, err := netip.ParseAddr(ip.raw)
		if err != nil || addr.Is4() != (ip.typ == "A") || !addr.IsGlobalUnicast() || addr.IsPrivate() {
			return "", nil, fmt.Errorf("%s address %q must be a public %s address", ip.typ, ip.raw, ip.typ)
		}
		if b.inZone(host) {
			b.add(KindHost, host, ip.typ, addr.String())
		}
	}
	switch {
	case !b.inZone(host):
		b.note(NoteInfo, "host_outside_zone", fmt.Sprintf("%s is outside this zone; publish its A/AAAA records where that name is hosted.", host))
	case spec.IPv4 == "" && spec.IPv6 == "":
		b.note(NoteWarn, "host_no_ip", fmt.Sprintf("No IP given, so no A/AAAA record for %s was built. The host must resolve before mail can be delivered.", host))
	}
	return host, []string{host}, nil
}

func (b *builder) spf(spec Spec, preset *Preset) error {
	mode := spec.SPFMode
	if mode == "" {
		mode = SPFServer
		if preset != nil {
			mode = SPFInclude
		}
	}
	if mode != SPFServer && mode != SPFInclude {
		return fmt.Errorf("spf_mode must be %q or %q", SPFServer, SPFInclude)
	}
	var tokens []string
	if mode == SPFServer {
		if preset != nil {
			return errors.New("spf_mode server cannot be used with a hosted preset; use include")
		}
		tokens = append(tokens, "mx")
		for _, ip := range []struct{ raw, prefix string }{{spec.IPv4, "ip4:"}, {spec.IPv6, "ip6:"}} {
			if ip.raw != "" {
				a, _ := netip.ParseAddr(ip.raw)
				tokens = append(tokens, ip.prefix+a.String())
			}
		}
	}
	incs := slices.Clone(spec.SPFIncludes)
	if preset != nil {
		incs = append([]string{preset.SPFInclude}, incs...)
	}
	for _, inc := range incs {
		h := strings.TrimPrefix(strings.TrimSpace(inc), "include:")
		// SPF include hosts such as _spf.example.net start labels with an underscore.
		if ValidHost(strings.ReplaceAll(h, "_", "a")) != nil {
			return fmt.Errorf("spf include %q is not a valid host name", inc)
		}
		if t := "include:" + h; !slices.Contains(tokens, t) {
			tokens = append(tokens, t)
		}
	}
	if mode == SPFInclude && len(incs) == 0 {
		return errors.New("spf_mode include needs at least one include host or a preset")
	}
	if n := len(slices.DeleteFunc(slices.Clone(tokens), func(t string) bool { return strings.HasPrefix(t, "ip") })); n > 10 {
		b.note(NoteWarn, "spf_lookups", "SPF may exceed the 10 DNS lookup limit; receivers treat that as a permanent error.")
	}
	b.add(KindSPF, b.domain, "TXT", "v=spf1 "+strings.Join(tokens, " ")+" ~all")
	return nil
}

func (b *builder) dkim(spec Spec, preset *Preset) error {
	if preset != nil {
		for _, c := range preset.DKIMCNAMEs {
			b.add(KindDKIM, c.Selector+"._domainkey."+b.domain, "CNAME", fmt.Sprintf(c.Target, b.domain))
		}
	}
	sel := spec.DKIMSelector
	if sel == "" && spec.DKIMPublicKey == "" {
		if preset == nil || len(preset.DKIMCNAMEs) == 0 {
			b.note(NoteWarn, "dkim_pending", "No DKIM key yet: no record was built. Generate a key pair on the mail server or provider, then re-run with the selector and public key.")
		}
		return nil
	}
	if !labelRE.MatchString(sel) {
		return fmt.Errorf("dkim_selector %q must be a valid DNS label", sel)
	}
	if spec.DKIMPublicKey == "" {
		b.note(NoteWarn, "dkim_pending", fmt.Sprintf("DKIM key for selector %s is pending: no record was built.", sel))
		return nil
	}
	val, bits, err := dkimValue(spec.DKIMPublicKey)
	if err != nil {
		return fmt.Errorf("dkim_public_key: %w", err)
	}
	if bits > 0 && bits < 2048 {
		b.note(NoteWarn, "dkim_weak", fmt.Sprintf("DKIM RSA key is %d bits; 2048 is recommended.", bits))
	}
	b.add(KindDKIM, sel+"._domainkey."+b.domain, "TXT", val)
	return nil
}

// dkimValue turns a PEM or bare base64 public key into a DKIM TXT value and
// reports the RSA modulus size (0 for other key types).
func dkimValue(raw string) (string, int, error) {
	var lines []string
	for _, l := range strings.Split(raw, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "-----") {
			lines = append(lines, l)
		}
	}
	b64 := strings.Join(strings.Fields(strings.Join(lines, "")), "")
	der, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", 0, errors.New("not valid base64; paste the public key only, never the private key")
	}
	pub, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return "", 0, errors.New("not a PKIX public key (a private key was probably pasted)")
	}
	switch k := pub.(type) {
	case *rsa.PublicKey:
		if k.N.BitLen() < 1024 {
			return "", 0, fmt.Errorf("RSA key is %d bits; at least 1024 is required", k.N.BitLen())
		}
		return "v=DKIM1; k=rsa; p=" + b64, k.N.BitLen(), nil
	case ed25519.PublicKey:
		return "v=DKIM1; k=ed25519; p=" + base64.StdEncoding.EncodeToString(k), 0, nil
	}
	return "", 0, errors.New("only RSA and Ed25519 keys are supported")
}

func (b *builder) dmarc(spec Spec) error {
	policy := strings.ToLower(spec.DMARCPolicy)
	if policy == "" {
		policy = "none"
	}
	if !slices.Contains([]string{"none", "quarantine", "reject"}, policy) {
		return errors.New("dmarc_policy must be none, quarantine or reject")
	}
	rua := spec.DMARCRua
	if rua == "" {
		rua = "dmarc@" + b.domain
	}
	addr, err := mail.ParseAddress(rua)
	if err != nil || addr.Address != rua || strings.ContainsAny(rua, ";, ") {
		return fmt.Errorf("dmarc_rua %q is not a plain email address", rua)
	}
	pct := spec.DMARCPct
	if pct == 0 {
		pct = 100
	}
	if pct < 1 || pct > 100 {
		return errors.New("dmarc_pct must be between 1 and 100")
	}
	v := "v=DMARC1; p=" + policy + "; rua=mailto:" + rua
	if pct != 100 {
		v += "; pct=" + strconv.Itoa(pct)
	}
	b.add(KindDMARC, "_dmarc."+b.domain, "TXT", v)
	if policy == "none" {
		b.note(NoteInfo, "dmarc_monitor", "DMARC policy none only collects reports. Move to quarantine or reject once SPF and DKIM pass.")
	}
	if !strings.HasSuffix(rua, "@"+b.domain) {
		b.note(NoteWarn, "dmarc_external_rua", "The report address is on another domain; that domain must publish a "+b.domain+"._report._dmarc TXT record authorizing reports.")
	}
	return nil
}

func (b *builder) transport(spec Spec, host string, mxHosts []string) error {
	if spec.TLSRPT {
		rua := spec.TLSRPTRua
		if rua == "" {
			rua = "tlsrpt@" + b.domain
		}
		if a, err := mail.ParseAddress(rua); err != nil || a.Address != rua || strings.ContainsAny(rua, ";, ") {
			return fmt.Errorf("tls_rpt_rua %q is not a plain email address", rua)
		}
		b.add(KindTLSRPT, "_smtp._tls."+b.domain, "TXT", "v=TLSRPTv1; rua=mailto:"+rua)
	}
	if spec.MTASTSMode != "" {
		if err := b.mtaSTS(spec, mxHosts); err != nil {
			return err
		}
	}
	if spec.Autoconfig {
		if host == "" {
			return errors.New("autoconfig needs your own mail host; hosted presets publish their own client settings")
		}
		b.add(KindAutoconfig, "autoconfig."+b.domain, "CNAME", host)
		b.add(KindAutoconfig, "autodiscover."+b.domain, "CNAME", host)
		b.add(KindAutoconfig, "_submission._tcp."+b.domain, "SRV", "0 1 587 "+host)
		b.add(KindAutoconfig, "_imaps._tcp."+b.domain, "SRV", "0 1 993 "+host)
		b.note(NoteInfo, "autoconfig_http", "The autoconfig and autodiscover names only help clients if the mail host serves the matching XML over HTTP(S); the SRV records (RFC 6186) work without it.")
	}
	return nil
}

func (b *builder) mtaSTS(spec Spec, mxHosts []string) error {
	if spec.MTASTSMode != "testing" && spec.MTASTSMode != "enforce" {
		return errors.New("mta_sts_mode must be testing or enforce")
	}
	age := spec.MTASTSMaxAge
	if age == 0 {
		age = defaultMTASTSMaxAge()
	}
	if age < 0 {
		return errors.New("mta_sts_max_age must be positive")
	}
	policy := "version: STSv1\nmode: " + spec.MTASTSMode + "\n"
	for _, h := range mxHosts {
		policy += "mx: " + h + "\n"
	}
	policy += "max_age: " + strconv.Itoa(age) + "\n"
	sum := sha256.Sum256([]byte(policy))
	b.add(KindMTASTS, "_mta-sts."+b.domain, "TXT", "v=STSv1; id="+hex.EncodeToString(sum[:8]))
	b.note(NoteInfo, "mta_sts_policy", "Serve this exact text over HTTPS at https://mta-sts."+b.domain+"/.well-known/mta-sts.txt with a valid certificate (this server does not host it):\n"+policy)
	return nil
}
