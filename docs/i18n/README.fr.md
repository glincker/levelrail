<!-- translated-from: README.md blob:b3c2e12bc506fb9fd5a89df18863898321998952 lang:fr -->
<div align="center">
<img src="../../docs/assets/brand/levelrail-default.svg" alt="Logo Levelrail" width="56" height="56">
<h1>Levelrail</h1>

> This translation was generated with AI and may contain mistakes. Corrections are welcome: see [docs/i18n/README.md](README.md).

<p><strong>Git push vers la production sur vos propres serveurs.</strong><br>
Une plateforme de déploiement auto-hébergée dans un seul binaire Go. Le plan de contrôle consomme au repos environ 200 Mo de RAM (<a href="../../docs/performance.md">mesuré</a>), avec 570+ apps en un clic, rollbacks, prévisualisations, HTTPS et bases de données intégrés.</p>
<p><code>curl -fsSL https://levelrail.com/install.sh | sudo sh</code> &middot; <a href="https://demo.levelrail.com"><b>Essayer la démo en ligne</b></a></p>
<p>
  <a href="https://github.com/glincker/levelrail/stargazers"><img src="https://img.shields.io/github/stars/glincker/levelrail?style=flat&logo=github" alt="Étoiles GitHub"></a>
  <a href="../../LICENSE"><img src="https://img.shields.io/github/license/glincker/levelrail" alt="Licence : Apache 2.0"></a>
  <a href="https://github.com/glincker/levelrail/releases"><img src="https://img.shields.io/github/v/release/glincker/levelrail?include_prereleases&label=release" alt="Dernière version"></a>
  <a href="https://github.com/glincker/levelrail/actions/workflows/ci.yml"><img src="https://github.com/glincker/levelrail/actions/workflows/ci.yml/badge.svg" alt="Statut de la CI"></a>
  <a href="https://demo.levelrail.com"><img src="https://img.shields.io/badge/demo-demo.levelrail.com-2f6feb" alt="Démo en ligne"></a>
</p>
<p>
  <a href="https://levelrail.com?utm_source=github&amp;utm_medium=readme&amp;utm_campaign=nav">Site web</a> &middot;
  <a href="https://docs.levelrail.com?utm_source=github&amp;utm_medium=readme&amp;utm_campaign=nav">Docs</a> &middot;
  <a href="#démarrage-rapide">Démarrage rapide</a> &middot;
  <a href="#ce-qui-distingue-levelrail">Comparaison</a> &middot;
  <a href="../../docs/faq.md">FAQ</a> &middot;
  <a href="../../docs/roadmap.md">Roadmap</a> &middot;
  <a href="https://github.com/glincker/levelrail/discussions">Discussions</a>
</p>
<!-- i18n-nav:start -->
<p><a href="../../README.md">English</a> &middot; <a href="README.zh-Hans.md">简体中文</a> &middot; <a href="README.es.md">Español</a> &middot; <a href="README.pt-BR.md">Português (BR)</a> &middot; <a href="README.de.md">Deutsch</a> &middot; Français &middot; <a href="README.ja.md">日本語</a></p>
<!-- i18n-nav:end -->
<img src="../../docs/assets/demos/product-tour.gif" alt="Visite du dashboard Levelrail : vue d'ensemble du parc, liste des apps, historique des déploiements et métriques intégrées" width="880">
</div>

v1.x. Le mode mono-serveur est prêt pour la production, le multi-nœud est en beta. Levelrail transforme des serveurs Linux en cloud privé et fonctionne avec les clusters Kubernetes et les comptes cloud que vous avez déjà. Chaque fonctionnalité porte une étiquette stable, beta ou expérimentale dans [Feature status](../../docs/feature-status.md).

## Démarrage rapide

Il vous faut un serveur Linux (amd64 ou arm64) avec systemd et les droits root. L'installateur ajoute Docker. Ouvrez les ports 80 et 443.

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

1. Ouvrez le lien de configuration affiché par l'installateur et choisissez un mot de passe. Cela crée le compte administrateur.
2. Dans le dashboard, copiez la ligne de connexion de la carte **Connect your CLI** et exécutez-la (elle ressemble à `levelrail-cli login --api-url http://127.0.0.1:8080 --code ABCD-EF23`).
3. Déployez votre première app :

```bash
levelrail-cli import https://github.com/your-org/your-app --deploy
```

En cas d'échec, exécutez `levelrail-cli doctor` et consultez [Installation](../../docs/installing.md#troubleshooting). Ensuite : [Les 15 premières minutes](../../docs/first-15-minutes.md). CLI uniquement : `brew install glincker/tap/levelrail-cli` ou `npx levelrail-cli --help`.

Envie d'un aperçu d'abord ? [Essayez la démo en ligne](https://demo.levelrail.com), une démo hébergée du dashboard en lecture seule avec des données fictives ([comment elle fonctionne](../../docs/run-a-read-only-demo.md)).

## Où l'utiliser

| Surface | Démarrer | Docs |
| --- | --- | --- |
| Dashboard web | Ouvrez le lien de configuration affiché par l'installateur | [Les 15 premières minutes](../../docs/first-15-minutes.md) |
| CLI | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/apple/default.svg" alt="Apple" title="Apple" width="20" height="20"> `brew install glincker/tap/levelrail-cli`, `npx levelrail-cli --help` ou `npm install --global levelrail-cli` | [Installation](../../docs/installing.md) |
| API HTTP | Jeton Bearer, `/api/v1` | [Référence de l'API](../../docs/api-reference.md) |
| Terraform et OpenTofu | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/terraform/default.svg" alt="Terraform" title="Terraform" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/opentofu/default.svg" alt="OpenTofu" title="OpenTofu" width="20" height="20"> `source = "glincker/levelrail"` | [Fournisseur Terraform](../../docs/terraform-provider.md) |
| MCP et agents de codage | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/model-context-protocol/default.svg" alt="MCP" title="MCP" width="20" height="20"> 259 outils | [MCP](../../docs/mcp.md) |
| API mobile (beta, désactivée par défaut) | À activer dans Settings, Beta features | [API mobile](../../docs/mobile-api.md) |
| Runtime de bureau sur Mac (expérimental) | VM Linux privée sur votre Mac | [Runtime de bureau](../../docs/desktop-runtime.md) |

## Ce que vous obtenez

La maturité suit [Feature status](../../docs/feature-status.md).

| Domaine | En une ligne | Compatible avec |
| --- | --- | --- |
| [Déploiement](../../docs/deploying-apps.md) | Git push depuis GitHub, GitLab, Bitbucket, Gitea ou Forgejo, rollouts conditionnés à la disponibilité, rollback en une étape, prévisualisations et canaries | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/github/default.svg" alt="GitHub" title="GitHub" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitlab/default.svg" alt="GitLab" title="GitLab" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/bitbucket/default.svg" alt="Bitbucket" title="Bitbucket" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitea/default.svg" alt="Gitea" title="Gitea" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/docker/default.svg" alt="Docker" title="Docker" width="20" height="20"> |
| [Templates](../../docs/template-catalog.md) | 570+ templates dans le catalogue, 346 avec une exécution enregistrée sur un moteur réel | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/postgresql/default.svg" alt="PostgreSQL" title="PostgreSQL" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/redis/default.svg" alt="Redis" title="Redis" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/grafana/default.svg" alt="Grafana" title="Grafana" width="20" height="20"> |
| [Edge et HTTPS](../../docs/domains-and-ingress.md) | Certificats automatiques, wildcards compris, pare-feu, listes de bots, en-têtes et redirections par domaine | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/letsencrypt/default.svg" alt="Let's Encrypt" title="Let's Encrypt" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/cloudflare/default.svg" alt="Cloudflare" title="Cloudflare" width="20" height="20"> |
| [Nœuds et clouds](../../docs/node-provisioning.md) | Rejoignez n'importe quel hôte Linux avec un jeton, provisionnez des nœuds chez 6 clouds (beta), clusters Kubernetes existants comme cibles (expérimental) | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/hetzner/default.svg" alt="Hetzner" title="Hetzner" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/digitalocean/default.svg" alt="DigitalOcean" title="DigitalOcean" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/aws/default.svg" alt="AWS" title="AWS" width="20" height="20"> |
| [Bases de données et sauvegardes](../../docs/managing-databases.md) | 13 moteurs, sauvegardes vers un stockage compatible S3, R2 ou le disque local, [exercices de restauration](../../docs/prove-your-backups-work.md) | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/postgresql/default.svg" alt="PostgreSQL" title="PostgreSQL" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/mysql/default.svg" alt="MySQL" title="MySQL" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/mariadb/default.svg" alt="MariaDB" title="MariaDB" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/mongodb/default.svg" alt="MongoDB" title="MongoDB" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/redis/default.svg" alt="Redis" title="Redis" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/aws-amazon-simple-storage-service/default.svg" alt="Amazon S3" title="Amazon S3" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/minio/default.svg" alt="MinIO" title="MinIO" width="20" height="20"> |
| [Observabilité](../../docs/observability.md) | Métriques intégrées, recherche dans les logs, alertes vers 18 types de canaux, ingestion OTLP, page de statut. Grafana lit les logs via une [API compatible Loki](../../docs/loki-api.md) | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/grafana/default.svg" alt="Grafana" title="Grafana" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/prometheus/default.svg" alt="Prometheus" title="Prometheus" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/slack/default.svg" alt="Slack" title="Slack" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/discord/default.svg" alt="Discord" title="Discord" width="20" height="20"> |
| [Sécurité et accès](../../docs/identity-and-access.md) | Connexion OAuth et OIDC, passkeys, TOTP, politiques IAM, journal d'audit à chaînage de hachages | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/github/default.svg" alt="GitHub" title="GitHub" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitlab/default.svg" alt="GitLab" title="GitLab" width="20" height="20"> |
| [Automatisation](../../docs/cli-reference.md) | CLI, API HTTP, [fournisseur Terraform et OpenTofu](../../docs/terraform-provider.md), 259 [outils MCP](../../docs/mcp.md) pour les agents de codage | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/terraform/default.svg" alt="Terraform" title="Terraform" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/opentofu/default.svg" alt="OpenTofu" title="OpenTofu" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/model-context-protocol/default.svg" alt="MCP" title="MCP" width="20" height="20"> |

<details>
<summary>Toutes les fonctionnalités de déploiement</summary>

- Déploiements par Git push ou webhook depuis GitHub, GitLab, Bitbucket, Gitea et Forgejo. Voir [Intégrations Git](../../docs/git-integrations.md) et [Déploiement par Git push](../../docs/git-push-deploy.md).
- Builds Dockerfile, Compose et Railpack (Railpack est en beta) avec une file d'attente de builds. Voir [Build Center](../../docs/build-center.md).
- Rollouts conditionnés à la disponibilité et rollback en une étape. Voir [Déploiements sans interruption](../../docs/zero-downtime-deploys.md) et [Rollback](../../docs/rollback.md).
- Canaries et tests A/B. Voir [Déploiements canary](../../docs/canary-deploys.md) et [Tests A/B](../../docs/split-testing.md).
- Environnements de prévisualisation par pull request : stable sur GitHub, beta sur GitLab et Bitbucket. Voir [Prévisualisations de déploiement](../../docs/deploy-previews.md).
- Approbations de déploiement et gels de déploiement. Voir [Sécurité des déploiements](../../docs/deploy-safety.md).
- Fonctions en Node, Python ou Go (beta). Voir [Fonctions](../../docs/functions.md).
</details>
<details>
<summary>Tous les détails sur les templates</summary>

- 574 entrées de catalogue (565 actives, 9 obsolètes) dans 22 catégories.
- 346 entrées ont une exécution enregistrée sur un moteur réel.
- 228 entrées sont marquées en prévisualisation : installables, sans exécution enregistrée pour l'instant.
</details>
<details>
<summary>Toutes les fonctionnalités d'edge</summary>

- Ingress embarqué avec TLS automatique via HTTP-01 et wildcards DNS-01.
- En-têtes, redirecteurs, règles géographiques, cache et redirections par domaine. Voir [Contrôles du trafic par domaine](../../docs/domain-traffic-controls.md).
- Règles de pare-feu, listes d'IP, défi navigateur, listes de bots et mode attaque. Voir [Pare-feu](../../docs/firewall.md).
- Limites de débit et listes de blocage par app (beta). Voir [Défense contre les abus](../../docs/abuse-defense.md).
- Répartition round-robin entre les réplicas. Voir [Répartition de charge](../../docs/load-balancing.md).
- Réseaux privés et maillage WireGuard (beta). Voir [Multi-nœud](../../docs/multi-node.md).
</details>
<details>
<summary>Toutes les fonctionnalités de nœuds et de clouds</summary>

- Provisionnez des nœuds chez Hetzner, DigitalOcean, AWS, Azure, GCP ou Oracle Cloud (beta).
- Rejoignez n'importe quel hôte Linux avec un jeton à usage unique via mTLS. Voir [Ajouter un nœud n'importe où](../../docs/add-node-anywhere.md).
- DNS chez Cloudflare et Route53, registres d'images sur ECR, Artifact Registry et ACR. Voir [Intégrations cloud](../../docs/cloud-integrations.md) pour le statut par cloud.
- Fonctionne avec les clusters et les comptes cloud que vous avez déjà (expérimental) : connectez des clusters Kubernetes existants et déployez dessus, désactivé par défaut. Voir [Clusters Kubernetes](../../docs/kubernetes-clusters.md).
</details>
<details>
<summary>Toutes les fonctionnalités de données et de sauvegardes</summary>

- 13 moteurs de base de données dans le code. 9 sont vérifiés en réel. 4 (Meilisearch, Qdrant, Typesense, Memcached) sont construits mais pas encore vérifiés en réel.
- Sauvegardes vers des cibles compatibles S3, R2 ou locales. Voir [Sauvegardes et stockage](../../docs/backups-and-storage.md).
- Exercices de restauration qui prouvent qu'une sauvegarde fonctionne. Voir [Prouvez que vos sauvegardes fonctionnent](../../docs/prove-your-backups-work.md).
- Récupération à un instant donné pour Postgres.
</details>
<details>
<summary>Toutes les fonctionnalités d'observabilité</summary>

- Métriques intégrées, recherche dans les logs et archive de logs. Voir [Explorateur de logs](../../docs/log-explorer.md) et [Archive de logs](../../docs/log-archive.md).
- Ingestion OTLP. Voir [Ingestion OTLP](../../docs/otlp-ingest.md).
- Page de statut publique. Voir [Page de statut](../../docs/status-page.md).
- Traces de déploiement et export du trafic. Voir [Trace de déploiement](../../docs/deploy-trace.md) et [Export du trafic](../../docs/traffic-export.md).
- Alertes vers 18 types de canaux de notification. Voir [Alertes](../../docs/alerts.md).
- Moniteurs de disponibilité HTTP, TCP, heartbeat et TLS (expérimental). Voir [Moniteurs](../../docs/monitors.md).
</details>
<details>
<summary>Toutes les fonctionnalités de sécurité et d'accès</summary>

- Mots de passe, passkeys et TOTP.
- Connexion OAuth avec 20 fournisseurs plus OIDC générique. Voir [OIDC](../../docs/sso-oidc.md).
- SAML et SCIM (beta, désactivés par défaut). Voir [SAML et SCIM](../../docs/sso-saml.md).
- Politiques IAM et équipes. Voir [Équipes et rôles](../../docs/teams-and-roles.md).
- Journal d'audit à chaînage de hachages. Voir [Journal d'audit](../../docs/audit-log.md).
- Secrets chiffrés. Voir [Environnement et secrets](../../docs/environment-and-secrets.md).
- Releases signées. Voir [Vérifier une release](../../docs/verify-a-release.md).
</details>
<details>
<summary>Toutes les fonctionnalités d'automatisation</summary>

- `levelrail-cli` et une API HTTP. Voir [Référence de l'API](../../docs/api-reference.md).
- Fournisseur Terraform et OpenTofu. Voir [Fournisseur Terraform](../../docs/terraform-provider.md), le [registre Terraform](https://registry.terraform.io/providers/glincker/levelrail/latest) et le [registre OpenTofu](https://search.opentofu.org/provider/glincker/levelrail/latest).
- Plateforme as code en YAML (expérimental). Voir [Platform as code](../../docs/platform-as-code.md).
- Pipelines de CI. Voir [Pipelines](../../docs/pipelines.md).
- 11 intégrations par app : Sentry, PostHog, GlitchTip, OneUptime, Plausible, Datadog, Axiom, Better Stack, LogSnag, Bugsnag et New Relic. Voir [Alertes](../../docs/alerts.md).
- Plugins signés pour les canaux de notification. Voir [Plugins](../../docs/plugins.md).
- 259 outils MCP pour les agents de codage. Voir [MCP](../../docs/mcp.md) et [Surface d'outils](../../docs/mcp-tool-surface.md).
- Un template de serveur de messagerie avec boîtes aux lettres et enregistrements DNS (expérimental). Voir [Template de messagerie](../../docs/mail-template.md).
</details>

Toutes les fonctionnalités, avec leurs étiquettes de maturité : [docs/features.md](../../docs/features.md).

## Conçu pour les agents de codage

259 [outils MCP](../../docs/mcp.md), limités aux capacités du jeton, avec confirmation obligatoire pour les écritures et audités. Ajoutez le skill avec `npx skills add https://github.com/glincker/levelrail --skill levelrail`, puis demandez à votre agent de déployer votre dépôt. Pour installer avec un agent, collez ceci dans Claude Code, Cursor ou tout agent capable d'exécuter des commandes shell sur votre serveur :

```text
Install Levelrail on this server: run `curl -fsSL https://levelrail.com/install.sh | sudo sh`.
Open the setup link it prints and tell me, because I choose the password myself.
Then run `levelrail-cli doctor` and `levelrail-cli status` and report anything that fails.
Do not write any token or password to a file or into this chat.
```

## Prêt pour la production, avec preuves

| Affirmation | Preuve |
| --- | --- |
| Chaque fonctionnalité étiquetée stable ou beta | [Feature status](../../docs/feature-status.md) |
| Résiste aux pannes | [Résilience](../../docs/resilience.md) |
| Mesuré, pas promis | [Benchmarks](../../docs/benchmarks.md) |
| Testé sur de vrais conteneurs | [CI](../../.github/workflows/ci.yml) et [`test/e2e`](../../test/e2e) |
| Releases signées | [Les vérifier](../../docs/installing.md#verifying-release-binaries) |
| Honnête sur les lacunes | [Limites connues](../../docs/known-limits.md) |

## Ce qui distingue Levelrail

Chaque affirmation renvoie à la page qui la prouve. La colonne de statut suit [Feature status](../../docs/feature-status.md).

| Ce que vous obtenez | Statut | Preuve |
| --- | --- | --- |
| Un plan de contrôle qui consomme environ 200 Mo de RAM au repos, dans un seul binaire Go | mesuré | [Performance](../../docs/performance.md), [Benchmarks](../../docs/benchmarks.md) |
| `git push` vers votre propre serveur en HTTPS, sans fournisseur, webhook ni clé SSH (les déploiements par webhook GitHub sont stables) | beta | [Déployer avec git push](../../docs/git-push-deploy.md) |
| Rollback sur images épinglées, déploiements conditionnés à la disponibilité et rollback automatique après une surveillance post-déploiement | stable (signal de latence beta) | [Sûreté des déploiements](../../docs/deploy-safety.md) |
| Sauvegardes vérifiées, avec des exercices de restauration dans une ressource temporaire | Postgres stable, autres moteurs beta | [Prouver que vos sauvegardes fonctionnent](../../docs/prove-your-backups-work.md), [Sauvegardes et stockage](../../docs/backups-and-storage.md) |
| Un journal d'audit chaîné par hachage que vous pouvez vérifier, filtrer et exporter | activé par défaut | [Journal d'audit](../../docs/audit-log.md) |
| Un agent de nœud qui se connecte au plan de contrôle en mTLS, donc les nœuds gérés n'ont besoin d'aucun port entrant | multi-nœud beta | [Architecture](../../docs/architecture.md), [Multi-nœud](../../docs/multi-node.md) |
| Un serveur MCP aux outils limités par capacités et aux écritures soumises à confirmation, jamais dans le chemin de réconciliation | beta | [MCP](../../docs/mcp.md), [Surface d'outils](../../docs/mcp-tool-surface.md) |

Venir d'une autre plateforme : [Migrer vers Levelrail](../../docs/migrate-to-levelrail.md). Notes d'architecture : [Comparaison](../../docs/comparison.md).

## Communauté

[Discussions](https://github.com/glincker/levelrail/discussions) &middot; [Issues](https://github.com/glincker/levelrail/issues) &middot; [Discord](https://discord.gg/Ar5pcaZB99) &middot; [support@levelrail.com](mailto:support@levelrail.com) &middot; [Politique de sécurité](../../.github/SECURITY.md)

Si Levelrail vous fait gagner un week-end, une [étoile](https://github.com/glincker/levelrail) aide d'autres auto-hébergeurs à le trouver.

## Licence

Apache 2.0, voir [LICENSE](../../LICENSE). Développé par Gagan Singh alias GDS K S de [GLINR Studios](https://glinr.com) et [theSVG](https://thesvg.org).

<div align="center">


</div>
