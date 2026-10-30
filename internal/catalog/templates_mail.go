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
var mailTemplates = []Template{
	{
		ID:     "mail-mox",
		Name:   "Mail server (mox, experimental)",
		Slogan: "Experimental, needs APP_EXPERIMENTAL=mail. Self-hosted mail server: SMTP, IMAP, webmail and an admin API in one small container (MIT).",
		Description: "Experimental. Needs APP_EXPERIMENTAL=mail on the control plane: without it this template is hidden and cannot be installed.\n" +
			"Set MAIL_FIRST_DOMAIN. MAIL_HOSTNAME (the FQDN other servers connect to) defaults to mail. that domain and MAIL_ADMIN_EMAIL to postmaster@ that domain.\n" +
			"Ports 25, 465, 587 and 993 bind privately by default: set a public bind for each in the app's Network settings and open them in the firewall. Webmail and the account UI are on port 8025 (private, no admin pages). The admin API and metrics are never published; never make them public.\n" +
			"TLS: the first start makes a self-signed certificate so the server comes up with no manual steps. Replace it before sending real mail: put your certificate and key in the config volume and set MAIL_TLS_CERT_FILE and MAIL_TLS_KEY_FILE (paths under /mox/config, for example certs/cur.pem and certs/cur.key). Existing files at those paths are never overwritten. A changed certificate needs a container restart.\n" +
			"Relay mode: set MAIL_SMARTHOST_HOST (and port, user, password) to send through a provider. Changing it needs a restart.\n" +
			"The admin password is the generated secret SERVICE_PASSWORD_MAILADMIN. Back up the config and data volumes together.\n" +
			"Deliverability depends on reverse DNS, SPF, DKIM and a clean IP; see the mail template guide. Licence: MIT.",
		Category:               "Communication",
		DocumentationURL:       "https://www.xmox.nl/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose:                mailMoxCompose(),
	},
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
  # Runs once per start and exits: makes a self-signed certificate only when the configured files are absent.
  mox-tls:
    image: alpine/openssl:3.5.4@sha256:42c7389ef077aed0eb4e96d0abbd094083d701bbaff1313073b061c0c9cd8278
    restart: "no"
    labels:
      levelrail.mail: tls-init
    mem_limit: 64m
    environment:
      MAIL_HOSTNAME: ${MAIL_HOSTNAME:-}
      MAIL_FIRST_DOMAIN: ${MAIL_FIRST_DOMAIN:?Set MAIL_FIRST_DOMAIN to the domain you want to receive mail for, for example example.com}
      MAIL_TLS_CERT_FILE: ${MAIL_TLS_CERT_FILE:-certs/selfsigned.pem}
      MAIL_TLS_KEY_FILE: ${MAIL_TLS_KEY_FILE:-certs/selfsigned.key}
    entrypoint: ["sh", "-c"]
    command:
      - |
        set -e
        cd /mox/config
        [ -n "$MAIL_HOSTNAME" ] || MAIL_HOSTNAME=mail.$MAIL_FIRST_DOMAIN
        if [ -f "$MAIL_TLS_CERT_FILE" ] && [ -f "$MAIL_TLS_KEY_FILE" ]; then exit 0; fi
        if [ -e "$MAIL_TLS_CERT_FILE" ] || [ -e "$MAIL_TLS_KEY_FILE" ]; then echo "mail template: only one of the certificate and key files exists, add the other or remove both" >&2; exit 1; fi
        mkdir -p "$(dirname "$MAIL_TLS_CERT_FILE")" "$(dirname "$MAIL_TLS_KEY_FILE")"
        openssl req -x509 -newkey rsa:2048 -nodes -days 3650 -subj "/CN=$MAIL_HOSTNAME" -addext "subjectAltName=DNS:$MAIL_HOSTNAME" -keyout "$MAIL_TLS_KEY_FILE" -out "$MAIL_TLS_CERT_FILE" 2>/dev/null
        chmod 640 "$MAIL_TLS_KEY_FILE"; chown 1000:1000 "$MAIL_TLS_CERT_FILE" "$MAIL_TLS_KEY_FILE"
        mkdir -p certs; touch certs/.selfsigned
        echo "mail template: made a self-signed certificate for $MAIL_HOSTNAME"
    volumes:
      - mail_config:/mox/config
  mox:
    image: r.xmox.nl/mox:v0.0.17@sha256:8889e7b48fdd5b0b07b51dd11a6d06536334ec442652f2ffb1e1df3a9d45074e
    labels:
      levelrail.mail: engine
    mem_limit: "@MEM@m"
    cpus: "@CPUS@"
    depends_on:
      mox-tls:
        condition: service_completed_successfully
    ports:
      - "8025:8081"
      - "25:25"
      - "465:465"
      - "587:587"
      - "993:993"
    environment:
      MAIL_HOSTNAME: ${MAIL_HOSTNAME:-}
      MAIL_FIRST_DOMAIN: ${MAIL_FIRST_DOMAIN:?Set MAIL_FIRST_DOMAIN to the domain you want to receive mail for, for example example.com}
      MAIL_ADMIN_EMAIL: ${MAIL_ADMIN_EMAIL:-}
      MAIL_ADMIN_PASSWORD: ${SERVICE_PASSWORD_MAILADMIN}
      MAIL_TLS_CERT_FILE: ${MAIL_TLS_CERT_FILE:-certs/selfsigned.pem}
      MAIL_TLS_KEY_FILE: ${MAIL_TLS_KEY_FILE:-certs/selfsigned.key}
      MAIL_SMARTHOST_HOST: ${MAIL_SMARTHOST_HOST:-}
      MAIL_SMARTHOST_PORT: ${MAIL_SMARTHOST_PORT:-587}
      MAIL_SMARTHOST_USER: ${MAIL_SMARTHOST_USER:-}
      MAIL_SMARTHOST_PASSWORD: ${MAIL_SMARTHOST_PASSWORD:-}
    entrypoint: ["sh", "-c"]
    command:
      - |
        set -e
        cd /mox
        mkdir -p config data
        [ -n "$MAIL_HOSTNAME" ] || MAIL_HOSTNAME=mail.$MAIL_FIRST_DOMAIN
        if [ ! -f "config/$MAIL_TLS_CERT_FILE" ] || [ ! -f "config/$MAIL_TLS_KEY_FILE" ]; then echo "mail template: the certificate or key file is missing from the config volume ($MAIL_TLS_CERT_FILE, $MAIL_TLS_KEY_FILE under /mox/config). Add both files or clear the two settings to use a self-signed certificate, then restart." >&2; exit 1; fi
        if [ -f config/certs/.selfsigned ] && [ "$MAIL_TLS_CERT_FILE" = certs/selfsigned.pem ]; then echo "mail template: using a self-signed certificate, replace it before sending real mail: put your certificate and key in the config volume, set MAIL_TLS_CERT_FILE and MAIL_TLS_KEY_FILE, and restart"; fi
        [ -n "$MAIL_ADMIN_EMAIL" ] || MAIL_ADMIN_EMAIL=postmaster@$MAIL_FIRST_DOMAIN
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
`
