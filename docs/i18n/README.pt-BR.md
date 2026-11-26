<!-- translated-from: README.md blob:b3c2e12bc506fb9fd5a89df18863898321998952 lang:pt-BR -->
<div align="center">
<p><a href="https://levelrail.com"><picture><source media="(prefers-color-scheme: dark)" srcset="../../docs/assets/brand/levelrail-lockup-dark.svg"><img src="../../docs/assets/brand/levelrail-lockup.svg" alt="Logo do Levelrail" height="72"></picture></a></p>

> This translation was generated with AI and may contain mistakes. Corrections are welcome: see [docs/i18n/README.md](README.md).
>
> Esta tradução foi gerada com IA e pode conter erros. Correções são bem-vindas: veja [docs/i18n/README.md](README.md).

<p><strong>Git push para produção nos seus próprios servidores.</strong><br>
Uma plataforma de deploy auto-hospedada em um único binário Go. O plano de controle consome cerca de 200 MB de RAM em repouso (<a href="../../docs/performance.md">medido</a>), com 570+ apps de um clique, rollbacks, pré-visualizações, HTTPS e bancos de dados integrados.</p>
<p><code>sudo sh install.sh</code> (<a href="https://github.com/glincker/levelrail/releases/latest">GitHub release</a>) &middot; <a href="https://demo.levelrail.com"><b>Experimente a demo ao vivo</b></a></p>
<p>
  <a href="https://github.com/glincker/levelrail/stargazers"><img src="https://img.shields.io/github/stars/glincker/levelrail?style=flat&logo=github" alt="Estrelas no GitHub"></a>
  <a href="../../LICENSE"><img src="https://img.shields.io/github/license/glincker/levelrail" alt="Licença: Apache 2.0"></a>
  <a href="https://github.com/glincker/levelrail/releases"><img src="https://img.shields.io/github/v/release/glincker/levelrail?include_prereleases&label=release" alt="Última versão"></a>
  <a href="https://github.com/glincker/levelrail/actions/workflows/ci.yml"><img src="https://github.com/glincker/levelrail/actions/workflows/ci.yml/badge.svg" alt="Status do CI"></a>
  <a href="https://demo.levelrail.com"><img src="https://img.shields.io/badge/demo-demo.levelrail.com-2f6feb" alt="Demo ao vivo"></a>
</p>
<p>
  <a href="https://levelrail.com?utm_source=github&amp;utm_medium=readme&amp;utm_campaign=nav">Site</a> &middot;
  <a href="https://docs.levelrail.com?utm_source=github&amp;utm_medium=readme&amp;utm_campaign=nav">Docs</a> &middot;
  <a href="#início-rápido">Início rápido</a> &middot;
  <a href="#o-que-diferencia-o-levelrail">Comparar</a> &middot;
  <a href="../../docs/faq.md">FAQ</a> &middot;
  <a href="../../docs/roadmap.md">Roadmap</a> &middot;
  <a href="https://github.com/glincker/levelrail/discussions">Discussões</a>
</p>
<!-- i18n-nav:start -->
<p><a href="../../README.md">English</a> &middot; <a href="README.zh-Hans.md">简体中文</a> &middot; <a href="README.es.md">Español</a> &middot; Português (BR) &middot; <a href="README.de.md">Deutsch</a> &middot; <a href="README.fr.md">Français</a> &middot; <a href="README.ja.md">日本語</a> &middot; <a href="README.ru.md">Русский</a></p>
<!-- i18n-nav:end -->
<img src="../../docs/assets/demos/product-tour.gif" alt="Tour do dashboard do Levelrail: visão geral da frota, lista de apps, histórico de deploys e métricas integradas" width="880">
</div>

v1.x. Um único servidor está pronto para produção, multi-nó é beta. O Levelrail transforma servidores Linux em uma nuvem privada e funciona com os clusters Kubernetes e as contas de nuvem que você já tem. Todo recurso traz um rótulo estável, beta ou experimental em [Status dos recursos](../../docs/feature-status.md).

## Início rápido

Você precisa de um servidor Linux (amd64 ou arm64) com systemd e acesso root. O instalador adiciona o Docker. Abra as portas 80 e 443.

```bash
curl -fsSLO https://github.com/glincker/levelrail/releases/latest/download/install.sh
curl -fsSLO https://github.com/glincker/levelrail/releases/latest/download/checksums.txt
grep ' install.sh$' checksums.txt | sha256sum -c -  # macOS: shasum -a 256 -c -
sudo sh install.sh
```

1. Abra o link de configuração que o instalador imprime e escolha uma senha. Isso cria a conta de administrador.
2. No dashboard, copie a linha de login do cartão **Connect your CLI** e execute-a (ela se parece com `levelrail-cli login --api-url http://127.0.0.1:8080 --code ABCD-EF23`).
3. Faça o deploy do seu primeiro app:

```bash
levelrail-cli import https://github.com/your-org/your-app --deploy
```

Se algo falhar, execute `levelrail-cli doctor` e veja [Instalação](../../docs/installing.md#troubleshooting). Em seguida: [Primeiros 15 minutos](../../docs/first-15-minutes.md). Só a CLI: `brew install glincker/tap/levelrail-cli` ou `npx levelrail-cli --help`.

Quer dar uma olhada antes? [Experimente a demo ao vivo](https://demo.levelrail.com), uma demo hospedada somente leitura do dashboard com dados fictícios ([como ela roda](../../docs/run-a-read-only-demo.md)).

## Onde você pode usá-lo

| Superfície | Início | Docs |
| --- | --- | --- |
| Dashboard web | Abra o link de configuração que o instalador imprime | [Primeiros 15 minutos](../../docs/first-15-minutes.md) |
| CLI | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/apple/default.svg" alt="Apple" title="Apple" width="20" height="20"> `brew install glincker/tap/levelrail-cli`, `npx levelrail-cli --help` ou `npm install --global levelrail-cli` | [Instalação](../../docs/installing.md) |
| API HTTP | Token Bearer, `/api/v1` | [Referência da API](../../docs/api-reference.md) |
| Terraform e OpenTofu | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/terraform/default.svg" alt="Terraform" title="Terraform" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/opentofu/default.svg" alt="OpenTofu" title="OpenTofu" width="20" height="20"> `source = "glincker/levelrail"` | [Provedor Terraform](../../docs/terraform-provider.md) |
| MCP e agentes de código | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/model-context-protocol/default.svg" alt="MCP" title="MCP" width="20" height="20"> 259 ferramentas | [MCP](../../docs/mcp.md) |
| API mobile (beta, desativada por padrão) | Ative em Settings, Beta features | [API mobile](../../docs/mobile-api.md) |
| Runtime desktop no Mac (experimental) | VM Linux privada no seu Mac | [Runtime desktop](../../docs/desktop-runtime.md) |

## O que você recebe

A maturidade segue o [Status dos recursos](../../docs/feature-status.md).

| Área | Em uma linha | Funciona com |
| --- | --- | --- |
| [Deploy](../../docs/deploying-apps.md) | Git push a partir do GitHub, GitLab, Bitbucket, Gitea ou Forgejo, rollouts com barreira de readiness, rollback em um passo, pré-visualizações e canaries | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/github/default.svg" alt="GitHub" title="GitHub" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitlab/default.svg" alt="GitLab" title="GitLab" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/bitbucket/default.svg" alt="Bitbucket" title="Bitbucket" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitea/default.svg" alt="Gitea" title="Gitea" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/docker/default.svg" alt="Docker" title="Docker" width="20" height="20"> |
| [Modelos](../../docs/template-catalog.md) | 570+ modelos no catálogo, 346 com uma execução registrada em um motor real | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/postgresql/default.svg" alt="PostgreSQL" title="PostgreSQL" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/redis/default.svg" alt="Redis" title="Redis" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/grafana/default.svg" alt="Grafana" title="Grafana" width="20" height="20"> |
| [Edge e HTTPS](../../docs/domains-and-ingress.md) | Certificados automáticos incluindo wildcards, firewall, listas de bots, cabeçalhos e redirecionamentos por domínio | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/letsencrypt/default.svg" alt="Let's Encrypt" title="Let's Encrypt" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/cloudflare/default.svg" alt="Cloudflare" title="Cloudflare" width="20" height="20"> |
| [Nós e nuvens](../../docs/node-provisioning.md) | Conecte qualquer host Linux com um token, provisione nós em 6 nuvens (beta), clusters Kubernetes existentes como alvos (experimental) | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/hetzner/default.svg" alt="Hetzner" title="Hetzner" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/digitalocean/default.svg" alt="DigitalOcean" title="DigitalOcean" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/aws/default.svg" alt="AWS" title="AWS" width="20" height="20"> |
| [Bancos de dados e backups](../../docs/managing-databases.md) | 13 motores, backups para S3-compatível, R2 ou disco local, [simulações de restauração](../../docs/prove-your-backups-work.md) | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/postgresql/default.svg" alt="PostgreSQL" title="PostgreSQL" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/mysql/default.svg" alt="MySQL" title="MySQL" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/mariadb/default.svg" alt="MariaDB" title="MariaDB" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/mongodb/default.svg" alt="MongoDB" title="MongoDB" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/redis/default.svg" alt="Redis" title="Redis" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/aws-amazon-simple-storage-service/default.svg" alt="Amazon S3" title="Amazon S3" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/minio/default.svg" alt="MinIO" title="MinIO" width="20" height="20"> |
| [Observabilidade](../../docs/observability.md) | Métricas integradas, busca de logs, alertas para 18 tipos de canal, ingestão OTLP, página de status. O Grafana lê logs por uma [API compatível com Loki](../../docs/loki-api.md) | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/grafana/default.svg" alt="Grafana" title="Grafana" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/prometheus/default.svg" alt="Prometheus" title="Prometheus" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/slack/default.svg" alt="Slack" title="Slack" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/discord/default.svg" alt="Discord" title="Discord" width="20" height="20"> |
| [Segurança e acesso](../../docs/identity-and-access.md) | Login com OAuth e OIDC, passkeys, TOTP, políticas IAM, log de auditoria encadeado por hash | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/github/default.svg" alt="GitHub" title="GitHub" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitlab/default.svg" alt="GitLab" title="GitLab" width="20" height="20"> |
| [Automação](../../docs/cli-reference.md) | CLI, API HTTP, [provedor Terraform e OpenTofu](../../docs/terraform-provider.md), 259 [ferramentas MCP](../../docs/mcp.md) para agentes de código | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/terraform/default.svg" alt="Terraform" title="Terraform" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/opentofu/default.svg" alt="OpenTofu" title="OpenTofu" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/model-context-protocol/default.svg" alt="MCP" title="MCP" width="20" height="20"> |

<details>
<summary>Todos os recursos de deploy</summary>

- Deploys por Git push ou webhook a partir de GitHub, GitLab, Bitbucket, Gitea e Forgejo. Veja [Integrações Git](../../docs/git-integrations.md) e [Deploy por Git push](../../docs/git-push-deploy.md).
- Builds com Dockerfile, Compose e Railpack (o Railpack é beta) com uma fila de builds. Veja [Build Center](../../docs/build-center.md).
- Rollouts com barreira de readiness e rollback em um passo. Veja [Deploys sem downtime](../../docs/zero-downtime-deploys.md) e [Rollback](../../docs/rollback.md).
- Canaries e testes divididos. Veja [Deploys canary](../../docs/canary-deploys.md) e [Testes divididos](../../docs/split-testing.md).
- Ambientes de pré-visualização por pull request: estável no GitHub, beta no GitLab e no Bitbucket. Veja [Pré-visualizações de deploy](../../docs/deploy-previews.md).
- Aprovações de deploy e congelamentos de deploy. Veja [Segurança de deploy](../../docs/deploy-safety.md).
- Funções em Node, Python ou Go (beta). Veja [Funções](../../docs/functions.md).
</details>
<details>
<summary>Todos os detalhes dos modelos</summary>

- 574 entradas de catálogo (565 ativas, 9 obsoletas) em 22 categorias.
- 346 entradas têm uma execução registrada em um motor real.
- 228 entradas estão marcadas como prévia: instaláveis, ainda sem execução registrada.
</details>
<details>
<summary>Todos os recursos de edge</summary>

- Ingress embutido com TLS automático via HTTP-01 e wildcards com DNS-01.
- Cabeçalhos, encaminhadores, regras geográficas, cache e redirecionamentos por domínio. Veja [Controles de tráfego por domínio](../../docs/domain-traffic-controls.md).
- Regras de firewall, listas de IP, desafio de navegador, listas de bots e modo de ataque. Veja [Firewall](../../docs/firewall.md).
- Limites de taxa e listas de bloqueio por app (beta). Veja [Defesa contra abuso](../../docs/abuse-defense.md).
- Balanceamento round-robin entre réplicas. Veja [Balanceamento de carga](../../docs/load-balancing.md).
- Redes privadas e uma malha WireGuard (beta). Veja [Multi-nó](../../docs/multi-node.md).
</details>
<details>
<summary>Todos os recursos de nós e nuvens</summary>

- Provisione nós na Hetzner, DigitalOcean, AWS, Azure, GCP ou Oracle Cloud (beta).
- Conecte qualquer host Linux com um token de uso único via mTLS. Veja [Adicione um nó em qualquer lugar](../../docs/add-node-anywhere.md).
- DNS na Cloudflare e no Route53, registries de imagens no ECR, Artifact Registry e ACR. Veja [Integrações de nuvem](../../docs/cloud-integrations.md) para o status por nuvem.
- Funciona com os clusters e as contas de nuvem que você já tem (experimental): conecte clusters Kubernetes existentes e faça deploy neles, desativado por padrão. Veja [Clusters Kubernetes](../../docs/kubernetes-clusters.md).
</details>
<details>
<summary>Todos os recursos de dados e backups</summary>

- 13 motores de banco de dados no código. 9 verificados ao vivo. 4 (Meilisearch, Qdrant, Typesense, Memcached) estão implementados, mas ainda não verificados ao vivo.
- Backups para destinos compatíveis com S3, R2 ou locais. Veja [Backups e armazenamento](../../docs/backups-and-storage.md).
- Simulações de restauração que provam que um backup funciona. Veja [Prove que seus backups funcionam](../../docs/prove-your-backups-work.md).
- Recuperação para um ponto no tempo no Postgres.
</details>
<details>
<summary>Todos os recursos de observabilidade</summary>

- Métricas integradas, busca de logs e arquivamento de logs. Veja [Explorador de logs](../../docs/log-explorer.md) e [Arquivo de logs](../../docs/log-archive.md).
- Ingestão OTLP. Veja [Ingestão OTLP](../../docs/otlp-ingest.md).
- Página de status pública. Veja [Página de status](../../docs/status-page.md).
- Traces de deploy e exportação de tráfego. Veja [Trace de deploy](../../docs/deploy-trace.md) e [Exportação de tráfego](../../docs/traffic-export.md).
- Alertas para 18 tipos de canal de notificação. Veja [Alertas](../../docs/alerts.md).
- Monitores de uptime HTTP, TCP, heartbeat e TLS (experimental). Veja [Monitores](../../docs/monitors.md).
</details>
<details>
<summary>Todos os recursos de segurança e acesso</summary>

- Senhas, passkeys e TOTP.
- Login com OAuth com 20 provedores mais OIDC genérico. Veja [OIDC](../../docs/sso-oidc.md).
- SAML e SCIM (beta, desativados por padrão). Veja [SAML e SCIM](../../docs/sso-saml.md).
- Políticas IAM e equipes. Veja [Equipes e papéis](../../docs/teams-and-roles.md).
- Log de auditoria encadeado por hash. Veja [Log de auditoria](../../docs/audit-log.md).
- Segredos criptografados. Veja [Ambiente e segredos](../../docs/environment-and-secrets.md).
- Versões assinadas. Veja [Verifique uma versão](../../docs/verify-a-release.md).
</details>
<details>
<summary>Todos os recursos de automação</summary>

- `levelrail-cli` e uma API HTTP. Veja [Referência da API](../../docs/api-reference.md).
- Provedor Terraform e OpenTofu. Veja [Provedor Terraform](../../docs/terraform-provider.md), o [registry do Terraform](https://registry.terraform.io/providers/glincker/levelrail/latest) e o [registry do OpenTofu](https://search.opentofu.org/provider/glincker/levelrail/latest).
- Plataforma como código em YAML (experimental). Veja [Plataforma como código](../../docs/platform-as-code.md).
- Pipelines de CI. Veja [Pipelines](../../docs/pipelines.md).
- 11 integrações por app: Sentry, PostHog, GlitchTip, OneUptime, Plausible, Datadog, Axiom, Better Stack, LogSnag, Bugsnag e New Relic. Veja [Alertas](../../docs/alerts.md).
- Plugins assinados para canais de notificação. Veja [Plugins](../../docs/plugins.md).
- 259 ferramentas MCP para agentes de código. Veja [MCP](../../docs/mcp.md) e [Superfície de ferramentas](../../docs/mcp-tool-surface.md).
- Um modelo de servidor de e-mail com caixas de correio e registros DNS (experimental). Veja [Modelo de e-mail](../../docs/mail-template.md).
</details>

Todos os recursos, com rótulos de maturidade: [docs/features.md](../../docs/features.md).

## Feito para agentes de código

259 [ferramentas MCP](../../docs/mcp.md), com escopo das permissões do token, com confirmação para escritas e auditadas. Adicione a skill com `npx skills add https://github.com/glincker/levelrail --skill levelrail`, depois peça ao seu agente para fazer o deploy do seu repositório. Para instalar com um agente, cole isto no Claude Code, Cursor ou qualquer agente que possa executar comandos de shell no seu servidor:

```text
Install Levelrail on this server: run `sudo sh install.sh` (download and verify it first, see [GitHub release](https://github.com/glincker/levelrail/releases/latest)).
Open the setup link it prints and tell me, because I choose the password myself.
Then run `levelrail-cli doctor` and `levelrail-cli status` and report anything that fails.
Do not write any token or password to a file or into this chat.
```

## Pronto para produção, com comprovações

| Afirmação | Evidência |
| --- | --- |
| Todo recurso rotulado como estável ou beta | [Status dos recursos](../../docs/feature-status.md) |
| Sobrevive a falhas | [Resiliência](../../docs/resilience.md) |
| Medido, não prometido | [Benchmarks](../../docs/benchmarks.md) |
| Testado com contêineres reais | [CI](../../.github/workflows/ci.yml) e [`test/e2e`](../../test/e2e) |
| Versões assinadas | [Verifique-as](../../docs/installing.md#verifying-release-binaries) |
| Honesto sobre as lacunas | [Limites conhecidos](../../docs/known-limits.md) |

## O que diferencia o Levelrail

Cada afirmação leva à página que a comprova. A coluna de status segue o [Status dos recursos](../../docs/feature-status.md).

| O que você recebe | Status | Evidência |
| --- | --- | --- |
| Um plano de controle que, ocioso, usa cerca de 200 MB de RAM, em um único binário Go | medido | [Desempenho](../../docs/performance.md), [Benchmarks](../../docs/benchmarks.md) |
| `git push` para o seu próprio servidor por HTTPS, sem provedor, webhook nem chave SSH (os deploys por webhook do GitHub são estáveis) | beta | [Deploy com git push](../../docs/git-push-deploy.md) |
| Rollback para imagens fixadas, rollouts com verificação de prontidão e rollback automático após a vigilância pós-deploy | estável (sinal de latência beta) | [Segurança de deploy](../../docs/deploy-safety.md) |
| Backups verificados, com simulações de restauração em um recurso temporário | Postgres estável, outros engines beta | [Prove que seus backups funcionam](../../docs/prove-your-backups-work.md), [Backups e armazenamento](../../docs/backups-and-storage.md) |
| Um log de auditoria encadeado por hash que você pode verificar, filtrar e exportar | ativado por padrão | [Log de auditoria](../../docs/audit-log.md) |
| Um agente de nó que se conecta ao plano de controle por mTLS, então os nós gerenciados não precisam de portas de entrada | multi-nó beta | [Arquitetura](../../docs/architecture.md), [Multi-nó](../../docs/multi-node.md) |
| Um servidor MCP com ferramentas limitadas por habilidades e escritas que exigem confirmação, nunca no caminho de reconciliação | beta | [MCP](../../docs/mcp.md), [Superfície de ferramentas](../../docs/mcp-tool-surface.md) |

Vindo de outra plataforma: [Migrar para o Levelrail](../../docs/migrate-to-levelrail.md). Notas de arquitetura: [Comparação](../../docs/comparison.md).

## Comunidade

[Discussões](https://github.com/glincker/levelrail/discussions) &middot; [Issues](https://github.com/glincker/levelrail/issues) &middot; [Discord](https://discord.gg/Ar5pcaZB99) &middot; [support@levelrail.com](mailto:support@levelrail.com) &middot; [Política de segurança](../../.github/SECURITY.md)

Se o Levelrail te poupar um fim de semana, uma [estrela](https://github.com/glincker/levelrail) ajuda outras pessoas que hospedam os próprios serviços a encontrá-lo.

## Licença

Apache 2.0, veja [LICENSE](../../LICENSE). Criado por Gagan Singh aka GDS K S da [GLINR Studios](https://glinr.com) e do [theSVG](https://thesvg.org).

<div align="center">


</div>
