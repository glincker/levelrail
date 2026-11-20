<!-- translated-from: README.md blob:b3c2e12bc506fb9fd5a89df18863898321998952 lang:de -->
<div align="center">
<img src="../../docs/assets/brand/levelrail-default.svg" alt="Levelrail-Logo" width="56" height="56">
<h1>Levelrail</h1>

> This translation was generated with AI and may contain mistakes. Corrections are welcome: see [docs/i18n/README.md](README.md).

<p><strong>Git push in die Produktion auf eigenen Servern.</strong><br>
Eine selbst gehostete Deployment-Plattform in einem Go-Binary. Die Control Plane läuft im Leerlauf mit etwa 200 MB RAM (<a href="../../docs/performance.md">gemessen</a>), mit 570+ Ein-Klick-Apps, Rollbacks, Vorschauen, HTTPS und Datenbanken von Haus aus.</p>
<p><code>curl -fsSL https://github.com/glincker/levelrail/releases/latest/download/install.sh | sudo sh</code> &middot; <a href="https://demo.levelrail.com"><b>Live-Demo ausprobieren</b></a></p>
<p>
  <a href="https://github.com/glincker/levelrail/stargazers"><img src="https://img.shields.io/github/stars/glincker/levelrail?style=flat&logo=github" alt="GitHub-Sterne"></a>
  <a href="../../LICENSE"><img src="https://img.shields.io/github/license/glincker/levelrail" alt="Lizenz: Apache 2.0"></a>
  <a href="https://github.com/glincker/levelrail/releases"><img src="https://img.shields.io/github/v/release/glincker/levelrail?include_prereleases&label=release" alt="Neuestes Release"></a>
  <a href="https://github.com/glincker/levelrail/actions/workflows/ci.yml"><img src="https://github.com/glincker/levelrail/actions/workflows/ci.yml/badge.svg" alt="CI-Status"></a>
  <a href="https://demo.levelrail.com"><img src="https://img.shields.io/badge/demo-demo.levelrail.com-2f6feb" alt="Live-Demo"></a>
</p>
<p>
  <a href="https://levelrail.com?utm_source=github&amp;utm_medium=readme&amp;utm_campaign=nav">Website</a> &middot;
  <a href="https://docs.levelrail.com?utm_source=github&amp;utm_medium=readme&amp;utm_campaign=nav">Doku</a> &middot;
  <a href="#schnellstart">Schnellstart</a> &middot;
  <a href="#was-levelrail-ausmacht">Vergleich</a> &middot;
  <a href="../../docs/faq.md">FAQ</a> &middot;
  <a href="../../docs/roadmap.md">Roadmap</a> &middot;
  <a href="https://github.com/glincker/levelrail/discussions">Discussions</a>
</p>
<!-- i18n-nav:start -->
<p><a href="../../README.md">English</a> &middot; <a href="README.zh-Hans.md">简体中文</a> &middot; <a href="README.es.md">Español</a> &middot; <a href="README.pt-BR.md">Português (BR)</a> &middot; Deutsch &middot; <a href="README.fr.md">Français</a> &middot; <a href="README.ja.md">日本語</a></p>
<!-- i18n-nav:end -->
<img src="../../docs/assets/demos/product-tour.gif" alt="Levelrail-Dashboard-Tour: Flottenübersicht, App-Liste, Deployment-Verlauf und eingebaute Metriken" width="880">
</div>

v1.x. Einzelserver-Betrieb ist produktionsreif, Multi-Node ist Beta. Levelrail macht aus Linux-Servern eine private Cloud und arbeitet mit den Kubernetes-Clustern und Cloud-Konten, die du bereits hast. Jede Funktion trägt im [Feature-Status](../../docs/feature-status.md) eine Kennzeichnung als stabil, Beta oder experimentell.

## Schnellstart

Voraussetzung ist ein Linux-Server (amd64 oder arm64) mit systemd und Root-Zugriff. Der Installer richtet Docker ein. Die Ports 80 und 443 öffnen.

```bash
curl -fsSL https://github.com/glincker/levelrail/releases/latest/download/install.sh | sudo sh
```

1. Den vom Installer ausgegebenen Setup-Link öffnen und ein Passwort wählen. Damit wird das Admin-Konto angelegt.
2. Im Dashboard die Login-Zeile aus der Karte **Connect your CLI** kopieren und ausführen (sie sieht aus wie `levelrail-cli login --api-url http://127.0.0.1:8080 --code ABCD-EF23`).
3. Die erste App deployen:

```bash
levelrail-cli import https://github.com/your-org/your-app --deploy
```

Wenn etwas fehlschlägt, `levelrail-cli doctor` ausführen und [Installation](../../docs/installing.md#troubleshooting) ansehen. Weiter geht es mit [Die ersten 15 Minuten](../../docs/first-15-minutes.md). Nur die CLI: `brew install glincker/tap/levelrail-cli` oder `npx levelrail-cli --help`.

Erst mal reinschauen? [Die Live-Demo ausprobieren](https://demo.levelrail.com), eine gehostete Nur-Lese-Demo des Dashboards mit fiktiven Daten ([so läuft sie](../../docs/run-a-read-only-demo.md)).

## Wo man es nutzen kann

| Oberfläche | Start | Doku |
| --- | --- | --- |
| Web-Dashboard | Den vom Installer ausgegebenen Setup-Link öffnen | [Die ersten 15 Minuten](../../docs/first-15-minutes.md) |
| CLI | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/apple/default.svg" alt="Apple" title="Apple" width="20" height="20"> `brew install glincker/tap/levelrail-cli`, `npx levelrail-cli --help` oder `npm install --global levelrail-cli` | [Installation](../../docs/installing.md) |
| HTTP-API | Bearer-Token, `/api/v1` | [API-Referenz](../../docs/api-reference.md) |
| Terraform und OpenTofu | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/terraform/default.svg" alt="Terraform" title="Terraform" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/opentofu/default.svg" alt="OpenTofu" title="OpenTofu" width="20" height="20"> `source = "glincker/levelrail"` | [Terraform-Provider](../../docs/terraform-provider.md) |
| MCP und Coding-Agents | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/model-context-protocol/default.svg" alt="MCP" title="MCP" width="20" height="20"> 259 Tools | [MCP](../../docs/mcp.md) |
| Mobile-API (Beta, standardmäßig aus) | In den Einstellungen unter Beta-Funktionen aktivieren | [Mobile-API](../../docs/mobile-api.md) |
| Desktop-Runtime auf dem Mac (experimentell) | Private Linux-VM auf dem Mac | [Desktop-Runtime](../../docs/desktop-runtime.md) |

## Funktionsumfang

Der Reifegrad folgt dem [Feature-Status](../../docs/feature-status.md).

| Bereich | In einer Zeile | Funktioniert mit |
| --- | --- | --- |
| [Deployment](../../docs/deploying-apps.md) | Git push von GitHub, GitLab, Bitbucket, Gitea oder Forgejo, Rollouts mit Readiness-Gate, Rollback in einem Schritt, Vorschauen und Canaries | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/github/default.svg" alt="GitHub" title="GitHub" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitlab/default.svg" alt="GitLab" title="GitLab" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/bitbucket/default.svg" alt="Bitbucket" title="Bitbucket" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitea/default.svg" alt="Gitea" title="Gitea" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/docker/default.svg" alt="Docker" title="Docker" width="20" height="20"> |
| [Vorlagen](../../docs/template-catalog.md) | 570+ Vorlagen im Katalog, 346 mit aufgezeichnetem Lauf auf einer echten Engine | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/postgresql/default.svg" alt="PostgreSQL" title="PostgreSQL" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/redis/default.svg" alt="Redis" title="Redis" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/grafana/default.svg" alt="Grafana" title="Grafana" width="20" height="20"> |
| [Edge und HTTPS](../../docs/domains-and-ingress.md) | Automatische Zertifikate einschließlich Wildcards, Firewall, Bot-Listen, Header und Weiterleitungen pro Domain | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/letsencrypt/default.svg" alt="Let's Encrypt" title="Let's Encrypt" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/cloudflare/default.svg" alt="Cloudflare" title="Cloudflare" width="20" height="20"> |
| [Knoten und Clouds](../../docs/node-provisioning.md) | Jeden Linux-Host per Token einbinden, Knoten bei 6 Clouds bereitstellen (Beta), vorhandene Kubernetes-Cluster als Ziele (experimentell) | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/hetzner/default.svg" alt="Hetzner" title="Hetzner" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/digitalocean/default.svg" alt="DigitalOcean" title="DigitalOcean" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/aws/default.svg" alt="AWS" title="AWS" width="20" height="20"> |
| [Datenbanken und Backups](../../docs/managing-databases.md) | 13 Engines, Backups auf S3-kompatiblen Speicher, R2 oder lokalen Datenträger, [Restore-Übungen](../../docs/prove-your-backups-work.md) | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/postgresql/default.svg" alt="PostgreSQL" title="PostgreSQL" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/mysql/default.svg" alt="MySQL" title="MySQL" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/mariadb/default.svg" alt="MariaDB" title="MariaDB" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/mongodb/default.svg" alt="MongoDB" title="MongoDB" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/redis/default.svg" alt="Redis" title="Redis" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/aws-amazon-simple-storage-service/default.svg" alt="Amazon S3" title="Amazon S3" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/minio/default.svg" alt="MinIO" title="MinIO" width="20" height="20"> |
| [Observability](../../docs/observability.md) | Eingebaute Metriken, Log-Suche, Alarme an 18 Kanalarten, OTLP-Ingest, Statusseite. Grafana liest Logs über eine [Loki-kompatible API](../../docs/loki-api.md) | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/grafana/default.svg" alt="Grafana" title="Grafana" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/prometheus/default.svg" alt="Prometheus" title="Prometheus" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/slack/default.svg" alt="Slack" title="Slack" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/discord/default.svg" alt="Discord" title="Discord" width="20" height="20"> |
| [Sicherheit und Zugriff](../../docs/identity-and-access.md) | OAuth- und OIDC-Anmeldung, Passkeys, TOTP, IAM-Richtlinien, hash-verketteter Audit-Log | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/github/default.svg" alt="GitHub" title="GitHub" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitlab/default.svg" alt="GitLab" title="GitLab" width="20" height="20"> |
| [Automatisierung](../../docs/cli-reference.md) | CLI, HTTP-API, [Terraform- und OpenTofu-Provider](../../docs/terraform-provider.md), 259 [MCP-Tools](../../docs/mcp.md) für Coding-Agents | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/terraform/default.svg" alt="Terraform" title="Terraform" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/opentofu/default.svg" alt="OpenTofu" title="OpenTofu" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/model-context-protocol/default.svg" alt="MCP" title="MCP" width="20" height="20"> |

<details>
<summary>Alle Deployment-Funktionen</summary>

- Deployments per Git push oder Webhook von GitHub, GitLab, Bitbucket, Gitea und Forgejo. Siehe [Git-Integrationen](../../docs/git-integrations.md) und [Git-push-Deployment](../../docs/git-push-deploy.md).
- Builds mit Dockerfile, Compose und Railpack (Railpack ist Beta) mit Build-Warteschlange. Siehe [Build Center](../../docs/build-center.md).
- Rollouts mit Readiness-Gate und Rollback in einem Schritt. Siehe [Deployments ohne Ausfallzeit](../../docs/zero-downtime-deploys.md) und [Rollback](../../docs/rollback.md).
- Canaries und Split-Tests. Siehe [Canary-Deployments](../../docs/canary-deploys.md) und [Split-Tests](../../docs/split-testing.md).
- Preview-Umgebungen pro Pull Request: stabil auf GitHub, Beta auf GitLab und Bitbucket. Siehe [Deploy-Vorschauen](../../docs/deploy-previews.md).
- Deploy-Freigaben und Deploy-Freezes. Siehe [Deploy-Sicherheit](../../docs/deploy-safety.md).
- Funktionen in Node, Python oder Go (Beta). Siehe [Funktionen](../../docs/functions.md).
</details>
<details>
<summary>Alle Details zu Vorlagen</summary>

- 574 Katalogeinträge (565 aktiv, 9 veraltet) in 22 Kategorien.
- 346 Einträge haben einen aufgezeichneten Lauf auf einer echten Engine.
- 228 Einträge sind als Vorschau markiert: installierbar, noch ohne aufgezeichneten Lauf.
</details>
<details>
<summary>Alle Edge-Funktionen</summary>

- Eingebetteter Ingress mit automatischem TLS über HTTP-01 und DNS-01-Wildcards.
- Header, Weiterleitungen, Geo-Regeln, Caching und Redirects pro Domain. Siehe [Traffic-Steuerung pro Domain](../../docs/domain-traffic-controls.md).
- Firewall-Regeln, IP-Listen, Browser-Challenge, Bot-Listen und Angriffsmodus. Siehe [Firewall](../../docs/firewall.md).
- Ratenbegrenzung und Sperrlisten pro App (Beta). Siehe [Missbrauchsabwehr](../../docs/abuse-defense.md).
- Round-Robin-Lastverteilung über Replikate. Siehe [Lastverteilung](../../docs/load-balancing.md).
- Private Netzwerke und ein WireGuard-Mesh (Beta). Siehe [Multi-Node](../../docs/multi-node.md).
</details>
<details>
<summary>Alle Knoten- und Cloud-Funktionen</summary>

- Knoten bei Hetzner, DigitalOcean, AWS, Azure, GCP oder Oracle Cloud bereitstellen (Beta).
- Jeden Linux-Host mit einem Einmal-Token über mTLS einbinden. Siehe [Knoten überall hinzufügen](../../docs/add-node-anywhere.md).
- DNS bei Cloudflare und Route53, Image-Registries auf ECR, Artifact Registry und ACR. Siehe [Cloud-Integrationen](../../docs/cloud-integrations.md) für den Status je Cloud.
- Arbeitet mit den Clustern und Cloud-Konten, die du bereits hast (experimentell): vorhandene Kubernetes-Cluster verbinden und darauf deployen, standardmäßig aus. Siehe [Kubernetes-Cluster](../../docs/kubernetes-clusters.md).
</details>
<details>
<summary>Alle Daten- und Backup-Funktionen</summary>

- 13 Datenbank-Engines im Code. 9 sind live verifiziert. 4 (Meilisearch, Qdrant, Typesense, Memcached) sind gebaut, aber noch nicht live verifiziert.
- Backups auf S3-kompatible, R2- oder lokale Ziele. Siehe [Backups und Speicher](../../docs/backups-and-storage.md).
- Restore-Übungen, die beweisen, dass ein Backup funktioniert. Siehe [Backups auf Funktion prüfen](../../docs/prove-your-backups-work.md).
- Point-in-Time-Recovery für Postgres.
</details>
<details>
<summary>Alle Observability-Funktionen</summary>

- Eingebaute Metriken, Log-Suche und Log-Archiv. Siehe [Log-Explorer](../../docs/log-explorer.md) und [Log-Archiv](../../docs/log-archive.md).
- OTLP-Ingest. Siehe [OTLP-Ingest](../../docs/otlp-ingest.md).
- Öffentliche Statusseite. Siehe [Statusseite](../../docs/status-page.md).
- Deploy-Traces und Traffic-Export. Siehe [Deploy-Trace](../../docs/deploy-trace.md) und [Traffic-Export](../../docs/traffic-export.md).
- Alarme an 18 Benachrichtigungskanalarten. Siehe [Alarme](../../docs/alerts.md).
- HTTP-, TCP-, Heartbeat- und TLS-Uptime-Monitore (experimentell). Siehe [Monitore](../../docs/monitors.md).
</details>
<details>
<summary>Alle Sicherheits- und Zugriffsfunktionen</summary>

- Passwörter, Passkeys und TOTP.
- OAuth-Anmeldung mit 20 Anbietern plus generisches OIDC. Siehe [OIDC](../../docs/sso-oidc.md).
- SAML und SCIM (Beta, standardmäßig aus). Siehe [SAML und SCIM](../../docs/sso-saml.md).
- IAM-Richtlinien und Teams. Siehe [Teams und Rollen](../../docs/teams-and-roles.md).
- Hash-verketteter Audit-Log. Siehe [Audit-Log](../../docs/audit-log.md).
- Verschlüsselte Secrets. Siehe [Umgebung und Secrets](../../docs/environment-and-secrets.md).
- Signierte Releases. Siehe [Ein Release verifizieren](../../docs/verify-a-release.md).
</details>
<details>
<summary>Alle Automatisierungsfunktionen</summary>

- `levelrail-cli` und eine HTTP-API. Siehe [API-Referenz](../../docs/api-reference.md).
- Terraform- und OpenTofu-Provider. Siehe [Terraform-Provider](../../docs/terraform-provider.md), die [Terraform-Registry](https://registry.terraform.io/providers/glincker/levelrail/latest) und die [OpenTofu-Registry](https://search.opentofu.org/provider/glincker/levelrail/latest).
- YAML-Platform-as-Code (experimentell). Siehe [Platform as Code](../../docs/platform-as-code.md).
- CI-Pipelines. Siehe [Pipelines](../../docs/pipelines.md).
- 11 Integrationen pro App: Sentry, PostHog, GlitchTip, OneUptime, Plausible, Datadog, Axiom, Better Stack, LogSnag, Bugsnag und New Relic. Siehe [Alarme](../../docs/alerts.md).
- Signierte Plugins für Benachrichtigungskanäle. Siehe [Plugins](../../docs/plugins.md).
- 259 MCP-Tools für Coding-Agents. Siehe [MCP](../../docs/mcp.md) und [Tool-Oberfläche](../../docs/mcp-tool-surface.md).
- Eine Mailserver-Vorlage mit Postfächern und DNS-Einträgen (experimentell). Siehe [Mail-Vorlage](../../docs/mail-template.md).
</details>

Alle Funktionen mit Reifegrad-Kennzeichnung: [docs/features.md](../../docs/features.md).

## Für Coding-Agents gebaut

259 [MCP-Tools](../../docs/mcp.md), auf die Fähigkeiten des Tokens begrenzt, bei Schreibzugriffen mit Bestätigung und auditiert. Den Skill mit `npx skills add https://github.com/glincker/levelrail --skill levelrail` hinzufügen und den Agent dann bitten, das eigene Repo zu deployen. Um mit einem Agent zu installieren, den folgenden Text in Claude Code, Cursor oder einen beliebigen Agent einfügen, der Shell-Befehle auf deinem Server ausführen kann:

```text
Install Levelrail on this server: run `curl -fsSL https://github.com/glincker/levelrail/releases/latest/download/install.sh | sudo sh`.
Open the setup link it prints and tell me, because I choose the password myself.
Then run `levelrail-cli doctor` and `levelrail-cli status` and report anything that fails.
Do not write any token or password to a file or into this chat.
```

## Produktionsreif, mit Belegen

| Aussage | Beleg |
| --- | --- |
| Jede Funktion als stabil oder Beta gekennzeichnet | [Feature-Status](../../docs/feature-status.md) |
| Übersteht Ausfälle | [Resilienz](../../docs/resilience.md) |
| Gemessen, nicht versprochen | [Benchmarks](../../docs/benchmarks.md) |
| Gegen echte Container getestet | [CI](../../.github/workflows/ci.yml) und [`test/e2e`](../../test/e2e) |
| Signierte Releases | [Verifizieren](../../docs/installing.md#verifying-release-binaries) |
| Ehrlich bei Lücken | [Bekannte Grenzen](../../docs/known-limits.md) |

## Was Levelrail ausmacht

Jede Aussage verlinkt auf die Seite, die sie belegt. Die Statusspalte folgt dem [Feature-Status](../../docs/feature-status.md).

| Was du bekommst | Status | Beleg |
| --- | --- | --- |
| Eine Control Plane, die im Leerlauf etwa 200 MB RAM braucht, in einem einzigen Go-Binary | gemessen | [Performance](../../docs/performance.md), [Benchmarks](../../docs/benchmarks.md) |
| `git push` zu deinem eigenen Server über HTTPS, ohne Anbieter, Webhook oder SSH-Schlüssel (GitHub-Webhook-Deploys sind stabil) | Beta | [Deploy mit git push](../../docs/git-push-deploy.md) |
| Rollback auf fixierte Images, Rollouts mit Readiness-Gate und automatischer Rollback durch eine Post-Deploy-Überwachung | stabil (Latenzsignal Beta) | [Deploy-Sicherheit](../../docs/deploy-safety.md) |
| Verifizierte Backups mit Restore-Drills in eine Scratch-Ressource | Postgres stabil, andere Engines Beta | [Backups beweisen](../../docs/prove-your-backups-work.md), [Backups und Speicher](../../docs/backups-and-storage.md) |
| Ein hash-verkettetes Audit-Log, das du verifizieren, filtern und exportieren kannst | standardmäßig an | [Audit-Log](../../docs/audit-log.md) |
| Ein Node-Agent, der per mTLS zur Control Plane verbindet, sodass verwaltete Knoten keine eingehenden Ports brauchen | Multi-Node Beta | [Architektur](../../docs/architecture.md), [Multi-Node](../../docs/multi-node.md) |
| Ein MCP-Server mit auf Fähigkeiten begrenzten Tools und Schreibzugriff nur nach Bestätigung, nie im Reconcile-Pfad | Beta | [MCP](../../docs/mcp.md), [Tool-Oberfläche](../../docs/mcp-tool-surface.md) |

Umstieg von einer anderen Plattform: [Zu Levelrail migrieren](../../docs/migrate-to-levelrail.md). Architekturnotizen: [Vergleich](../../docs/comparison.md).

## Community

[Discussions](https://github.com/glincker/levelrail/discussions) &middot; [Issues](https://github.com/glincker/levelrail/issues) &middot; [Discord](https://discord.gg/Ar5pcaZB99) &middot; [support@levelrail.com](mailto:support@levelrail.com) &middot; [Sicherheitsrichtlinie](../../.github/SECURITY.md)

Wenn Levelrail ein Wochenende spart, hilft ein [Stern](https://github.com/glincker/levelrail) anderen Selbsthostern, es zu finden.

## Lizenz

Apache 2.0, siehe [LICENSE](../../LICENSE). Entwickelt von Gagan Singh aka GDS K S von [GLINR Studios](https://glinr.com) und [theSVG](https://thesvg.org).

<div align="center">


</div>
