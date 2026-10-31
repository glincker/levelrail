package catalog

import (
	"os"
	"strconv"
	"strings"
)

const (
	mailDefaultMemoryMB = 256
	mailDefaultCPUs     = "1"
)

// mailTemplates holds the experimental in-house mail engine (ADR 058). The
// control plane adapter in internal/mail talks to its private admin API.
var mailTemplates = []Template{mailMoxTemplate()}

func mailMoxTemplate() Template {
	base := Template{
		ID:     "mail-mox",
		Name:   "Mail server (mox, experimental)",
		Slogan: "Experimental, needs APP_EXPERIMENTAL=mail. Self-hosted mail server: SMTP, IMAP, webmail and an admin API in one small container (MIT).",
		Description: "Experimental. Needs APP_EXPERIMENTAL=mail on the control plane: without it this template is hidden and cannot be installed.\n" +
			"Install with the MAIL_FIRST_DOMAIN setting (templates install mail-mox --env MAIL_FIRST_DOMAIN=example.com). MAIL_HOSTNAME (the FQDN other servers connect to) defaults to mail. that domain and MAIL_ADMIN_EMAIL to postmaster@ that domain.\n" +
			"Ports 25, 465, 587 and 993 are published on the host with a private bind by default. After install, open them with the CLI command mail ports open --confirm (or Open mail ports on the Mail page): that makes only those four ports public and allows them in the host firewall, then set up DNS. Webmail and the account UI are on port 8025 (private, no admin pages). The admin API and metrics are never published; never make them public.\n" +
			"TLS: the first start makes a self-signed certificate (valid for localhost only, made by mox itself) so the server comes up with no manual steps. Replace it before sending real mail: put your certificate and key in the config volume and set MAIL_TLS_CERT_FILE and MAIL_TLS_KEY_FILE (paths under /mox/config, for example certs/cur.pem and certs/cur.key). Existing files at those paths are never overwritten. A changed certificate needs a container restart.\n" +
			"Relay mode: set MAIL_SMARTHOST_HOST (and port, user, password) to send through a provider. Changing it needs a restart.\n" +
			"The admin password is the generated secret SERVICE_PASSWORD_MAILADMIN. Back up the config and data volumes together.\n" +
			"Deliverability depends on reverse DNS, SPF, DKIM and a clean IP; see the mail template guide. Licence: MIT.",
		Category:               "Communication",
		DocumentationURL:       "https://www.xmox.nl/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose:                mailMoxCompose(),
	}
	m := Manifest{
		ID: base.ID, Name: base.Name, Description: base.Description, Category: base.Category,
		DocumentationURL: base.DocumentationURL, RecommendedMemoryBytes: base.RecommendedMemoryBytes,
		Compose: base.Compose, Env: mailMoxPrompts(),
	}
	if err := m.Validate(); err != nil {
		panic(err)
	}
	t, err := m.Template()
	if err != nil {
		panic(err)
	}
	return t
}

// Defaults for the optional settings live in the start scripts, because the
// engine only resolves SERVICE_* variables, not shell-style defaults.
func mailMoxPrompts() []EnvPrompt {
	return []EnvPrompt{
		{Key: "MAIL_FIRST_DOMAIN", Label: "Mail domain", Required: true, Description: "The domain you want to receive mail for, for example example.com. Addresses will look like you@example.com."},
		{Key: "MAIL_HOSTNAME", Label: "Mail server host name", Description: "The name other mail servers connect to. Leave empty to use mail. plus your domain (mail.example.com). It needs a DNS record and matching reverse DNS."},
		{Key: "MAIL_ADMIN_EMAIL", Label: "Admin address", Description: "Leave empty to use postmaster@ plus your domain."},
		{Key: "MAIL_TLS_CERT_FILE", Label: "Certificate file", Description: "Optional path under the config volume, for example certs/cur.pem. Leave empty and the first start makes a self-signed certificate for testing, valid for localhost only."},
		{Key: "MAIL_TLS_KEY_FILE", Label: "Certificate key file", Description: "Optional path under the config volume, for example certs/cur.key. Set it together with the certificate file."},
		{Key: "MAIL_SMARTHOST_HOST", Label: "Relay host", Description: "Optional. Send outgoing mail through a provider instead of directly."},
		{Key: "MAIL_SMARTHOST_PORT", Label: "Relay port", Description: "Leave empty for 587. Use 465 for implicit TLS."},
		{Key: "MAIL_SMARTHOST_USER", Label: "Relay user"},
		{Key: "MAIL_SMARTHOST_PASSWORD", Label: "Relay password", Secret: true},
	}
}

// mailMoxCompose renders the resource defaults from env at load time so an
// operator can size small nodes without editing the template.
func mailMoxCompose() string {
	mem := mailDefaultMemoryMB
	if n, err := strconv.Atoi(os.Getenv("APP_MAIL_DEFAULT_MEMORY_MB")); err == nil && n > 0 {
		mem = n
	}
	cpus := mailDefaultCPUs
	if v := os.Getenv("APP_MAIL_DEFAULT_CPUS"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			cpus = v
		}
	}
	r := strings.NewReplacer("@MEM@", strconv.Itoa(mem), "@CPUS@", cpus)
	return r.Replace(mailMoxComposeBody)
}

// The init script runs as root, builds mox.conf from the inputs on every
// start (domains.conf belongs to the control plane), then mox drops to uid 1000.
// Config files use tabs; the awk step turns two-space levels into tabs.
const mailMoxComposeBody = `# Mail ports serve SMTP and IMAP clients directly, not the HTTP proxy. Setup notes are in the description.
services:
  mox:
    image: r.xmox.nl/mox:v0.0.17@sha256:8889e7b48fdd5b0b07b51dd11a6d06536334ec442652f2ffb1e1df3a9d45074e
    restart: unless-stopped
    labels:
      levelrail.mail: engine
    mem_limit: "@MEM@m"
    cpus: "@CPUS@"
    ports:
      - "8025:8081"
      - "25:25"
      - "465:465"
      - "587:587"
      - "993:993"
    # Published on their own host ports with a private bind; the web port stays routed and private.
    x-levelrail-published-ports: [25, 465, 587, 993]
    environment:
      MAIL_HOSTNAME: ${MAIL_HOSTNAME}
      MAIL_FIRST_DOMAIN: ${MAIL_FIRST_DOMAIN}
      MAIL_ADMIN_EMAIL: ${MAIL_ADMIN_EMAIL}
      MAIL_ADMIN_PASSWORD: ${SERVICE_PASSWORD_MAILADMIN}
      MAIL_TLS_CERT_FILE: ${MAIL_TLS_CERT_FILE}
      MAIL_TLS_KEY_FILE: ${MAIL_TLS_KEY_FILE}
      MAIL_SMARTHOST_HOST: ${MAIL_SMARTHOST_HOST}
      MAIL_SMARTHOST_PORT: ${MAIL_SMARTHOST_PORT}
      MAIL_SMARTHOST_USER: ${MAIL_SMARTHOST_USER}
      MAIL_SMARTHOST_PASSWORD: ${MAIL_SMARTHOST_PASSWORD}
    entrypoint: ["sh", "-c"]
    command:
      - |
        set -e
        cd /mox
        mkdir -p config data
        [ -n "$MAIL_FIRST_DOMAIN" ] || { echo "mail template: the mail domain is not set. Install with: levelrail-cli templates install mail-mox --env MAIL_FIRST_DOMAIN=example.com (or fill in Mail domain in the dashboard install form)" >&2; exit 1; }
        : "${MAIL_HOSTNAME:=mail.$MAIL_FIRST_DOMAIN}" "${MAIL_TLS_CERT_FILE:=certs/selfsigned.pem}" "${MAIL_TLS_KEY_FILE:=certs/selfsigned.key}" "${MAIL_SMARTHOST_PORT:=587}"
        have_c=0; have_k=0; [ -f "config/$MAIL_TLS_CERT_FILE" ] && have_c=1; [ -f "config/$MAIL_TLS_KEY_FILE" ] && have_k=1
        if [ "$have_c$have_k" = 00 ] && [ "$MAIL_TLS_CERT_FILE" = certs/selfsigned.pem ] && [ "$MAIL_TLS_KEY_FILE" = certs/selfsigned.key ]; then
          rm -rf /tmp/cert; mox localserve -initonly -dir /tmp/cert >/dev/null 2>&1 || { echo "mail template: could not make a self-signed certificate, put your certificate and key in the config volume and set MAIL_TLS_CERT_FILE and MAIL_TLS_KEY_FILE" >&2; exit 1; }
          mkdir -p config/certs; cp /tmp/cert/localhost.crt "config/$MAIL_TLS_CERT_FILE"; cp /tmp/cert/localhost.key "config/$MAIL_TLS_KEY_FILE"
          chmod 640 "config/$MAIL_TLS_KEY_FILE"; touch config/certs/.selfsigned; rm -rf /tmp/cert
        fi
        if [ ! -f "config/$MAIL_TLS_CERT_FILE" ] || [ ! -f "config/$MAIL_TLS_KEY_FILE" ]; then echo "mail template: the certificate or key file is missing from the config volume ($MAIL_TLS_CERT_FILE, $MAIL_TLS_KEY_FILE under /mox/config). Add both files or clear both settings, then restart." >&2; exit 1; fi
        : "${MAIL_ADMIN_EMAIL:=postmaster@$MAIL_FIRST_DOMAIN}"
        if [ ! -f config/.initialized ]; then
          if [ -e config/mox.conf ] || [ -e config/domains.conf ] || [ -e config/adminpasswd ]; then echo "mail template: mox config exists but was never initialised, refusing to overwrite" >&2; exit 1; fi
          rm -rf /tmp/init; mkdir /tmp/init; cd /tmp/init
          mox quickstart -skipdial -hostname $MAIL_HOSTNAME $MAIL_ADMIN_EMAIL 1000 </dev/null >/tmp/init/quickstart.out 2>&1 || { tail -n 20 /tmp/init/quickstart.out >&2; exit 1; }
          cp -a config/. /mox/config/
          cp -a data/. /mox/data/
          cp /mox/config/mox.conf /mox/config/mox.conf.quickstart
          touch /mox/config/.initialized
          cd /mox
        fi
        {
          if [ -n "$MAIL_SMARTHOST_HOST" ]; then
            if [ "$MAIL_SMARTHOST_PORT" = 465 ]; then mode=Submissions; else mode=Submission; fi
            printf 'Transports:\n  relay1:\n    %s:\n      Host: %s\n      Port: %s\n' $mode $MAIL_SMARTHOST_HOST $MAIL_SMARTHOST_PORT
            if [ -n "$MAIL_SMARTHOST_USER" ]; then
              printf '      Auth:\n        Username: %s\n        Password: "%s"\n' $MAIL_SMARTHOST_USER "$MAIL_SMARTHOST_PASSWORD"
            fi
          fi
          printf 'Listeners:\n  admin:\n    IPs:\n      - 0.0.0.0\n    Hostname: localhost\n    AdminHTTP:\n      Enabled: true\n      Port: 8080\n    MetricsHTTP:\n      Enabled: true\n      Port: 8010\n'
          printf '  web:\n    IPs:\n      - 0.0.0.0\n    Hostname: localhost\n    AccountHTTP:\n      Enabled: true\n      Port: 8081\n    WebmailHTTP:\n      Enabled: true\n      Port: 8081\n    WebAPIHTTP:\n      Enabled: true\n      Port: 8081\n'
          printf '  public:\n    IPs:\n      - 0.0.0.0\n    TLS:\n'
          printf '      KeyCerts:\n        -\n          CertFile: %s\n          KeyFile: %s\n' $MAIL_TLS_CERT_FILE $MAIL_TLS_KEY_FILE
          printf '    SMTP:\n      Enabled: true\n    Submission:\n      Enabled: true\n    Submissions:\n      Enabled: true\n    IMAPS:\n      Enabled: true\n    AutoconfigHTTPS:\n      Enabled: true\n    MTASTSHTTPS:\n      Enabled: true\n'
        } | awk '{ match($0, /^ */); s = ""; for (i = 0; i < RLENGTH / 2; i++) s = s "\t"; print s substr($0, RLENGTH + 1) }' > /tmp/mox.gen
        # Keep quickstart's own keys (paths, Postmaster, TLS reports); only listeners, ACME and transports are ours
        awk '/^[A-Za-z]/ { p = ($1 != "ACME:" && $1 != "Listeners:") } /^[ \t]*#/ { next } p' config/mox.conf.quickstart | sed 's/^LogLevel:.*/LogLevel: info/' > config/mox.conf
        cat /tmp/mox.gen >> config/mox.conf
        if [ -n "$MAIL_SMARTHOST_HOST" ] && ! grep -q '^Routes:' config/domains.conf; then
          printf 'Routes:\n\t-\n\t\tTransport: relay1\n' >> config/domains.conf
        fi
        printf '%s' "$MAIL_ADMIN_PASSWORD" | mox setadminpassword
        find /mox/config /mox/data -maxdepth 0 -user 1000 | grep -q . || chown -R 1000:1000 /mox/config /mox/data
        chown 1000:1000 config/mox.conf config/adminpasswd config/domains.conf
        chmod 640 config/mox.conf
        mox config test
        exec mox serve
    volumes:
      - mail_config:/mox/config
      - mail_data:/mox/data
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O /dev/null http://127.0.0.1:8080/admin/ && nc -z 127.0.0.1 25"]
      interval: 15s
      timeout: 5s
      retries: 5
      start_period: 30s
    # The routed web listener answers 403 on /admin/ by design, so the platform probes /webmail/ there.
    x-levelrail-healthcheck:
      kind: http
      path: /webmail/
      port: 8081
      interval: 15s
      timeout: 5s
      retries: 5
      start_period: 30s
`
