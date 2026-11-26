<!-- translated-from: README.md blob:9dd067f925253779cbff6f7fd0aa64784a5e9ec3 lang:ru -->
<div align="center">
<p><a href="https://levelrail.com"><picture><source media="(prefers-color-scheme: dark)" srcset="../../docs/assets/brand/levelrail-lockup-dark.svg"><img src="../../docs/assets/brand/levelrail-lockup.svg" alt="Логотип Levelrail" height="72"></picture></a></p>

> Этот перевод выполнен с помощью ИИ и может содержать ошибки. Исправления приветствуются: см. [docs/i18n/README.md](README.md).
<p><strong>Git push в продакшен на ваших серверах.</strong><br>
Самохостируемая платформа деплоя в одном Go-бинарнике. Плоскость управления в простое занимает около 200 МБ ОЗУ (<a href="../../docs/performance.md">замеры</a>), а 570+ приложений в один клик, откаты, превью, HTTPS и базы данных уже встроены.</p>
<p><code>brew install glincker/tap/levelrail-cli</code> &middot; <a href="#установка"><b>Установка</b></a> &middot; <a href="https://demo.levelrail.com"><b>Живая демо</b></a></p>
<p>
  <a href="https://github.com/glincker/levelrail/stargazers"><img src="https://img.shields.io/github/stars/glincker/levelrail?style=flat&logo=github" alt="Звезды на GitHub"></a>
  <a href="../../LICENSE"><img src="https://img.shields.io/github/license/glincker/levelrail" alt="Лицензия: Apache 2.0"></a>
  <a href="https://github.com/glincker/levelrail/releases"><img src="https://img.shields.io/github/v/release/glincker/levelrail?include_prereleases&label=release" alt="Последний релиз"></a>
  <a href="https://github.com/glincker/levelrail/actions/workflows/ci.yml"><img src="https://github.com/glincker/levelrail/actions/workflows/ci.yml/badge.svg" alt="Статус CI"></a>
  <a href="https://demo.levelrail.com"><img src="https://img.shields.io/badge/demo-demo.levelrail.com-2f6feb" alt="Живая демо"></a>
</p>
<p>
  <a href="https://levelrail.com?utm_source=github&amp;utm_medium=readme&amp;utm_campaign=nav">Сайт</a> &middot;
  <a href="https://docs.levelrail.com?utm_source=github&amp;utm_medium=readme&amp;utm_campaign=nav">Документация</a> &middot;
  <a href="#установка">Установка</a> &middot;
  <a href="#быстрый-старт">Быстрый старт</a> &middot;
  <a href="#чем-levelrail-выделяется">Почему Levelrail</a> &middot;
  <a href="../../docs/faq.md">FAQ</a> &middot;
  <a href="../../docs/roadmap.md">Дорожная карта</a> &middot;
  <a href="https://github.com/glincker/levelrail/discussions">Обсуждения</a>
</p>
<!-- i18n-nav:start -->
<p><a href="../../README.md">English</a> &middot; <a href="README.zh-Hans.md">简体中文</a> &middot; <a href="README.es.md">Español</a> &middot; <a href="README.pt-BR.md">Português (BR)</a> &middot; <a href="README.de.md">Deutsch</a> &middot; <a href="README.fr.md">Français</a> &middot; <a href="README.ja.md">日本語</a> &middot; Русский</p>
<!-- i18n-nav:end -->
<img src="../../docs/assets/demos/product-tour.gif" alt="Обзор панели Levelrail: парк серверов, список приложений, история деплоев и встроенные метрики" width="880">
</div>

v1.x. Один сервер готов к продакшену, несколько узлов в бете. Levelrail превращает Linux-серверы в частное облако и работает с уже имеющимися у вас кластерами Kubernetes и облачными аккаунтами. Каждая функция помечена как стабильная, бета или экспериментальная в [статусе функций](../../docs/feature-status.md).

## Установка

Выберите один вариант. Основной источник: ресурсы релиза на GitHub, где с каждым [релизом](https://github.com/glincker/levelrail/releases/latest) идут все бинарники, `install.sh`, `install-cli.sh` и `checksums.txt`.

- **Homebrew (CLI):** `brew install glincker/tap/levelrail-cli`
- **Docker (сервер):** `docker pull ghcr.io/glincker/levelrail:latest` (полную команду запуска с томами и портами см. в разделе [Установка](../../docs/installing.md))
- **Загрузка релиза с GitHub:** возьмите `install.sh` из [последнего релиза](https://github.com/glincker/levelrail/releases/latest) и запустите от root на Linux-сервере (amd64 или arm64, systemd). Установщик добавляет Docker.

```bash
curl -fsSLO https://github.com/glincker/levelrail/releases/latest/download/install.sh
sudo sh install.sh
```

<details>
<summary>Дополнительно: сначала проверьте установщик по checksums.txt</summary>

Скачайте, проверьте и запустите тремя отдельными шагами, без передачи в оболочку через пайп.

```bash
curl -fsSLO https://github.com/glincker/levelrail/releases/latest/download/install.sh
curl -fsSLO https://github.com/glincker/levelrail/releases/latest/download/checksums.txt
grep ' install.sh$' checksums.txt | sha256sum -c -
sudo sh install.sh
```

На macOS вместо `sha256sum -c -` используйте `shasum -a 256 -c -`. Файл `checksums.txt` подписан через cosign keyless (рядом лежит `checksums.txt.sigstore.json`). Как проверить подпись, см. [Установка](../../docs/installing.md#verifying-release-binaries).

</details>

## Быстрый старт

Нужен Linux-сервер (amd64 или arm64) с systemd и правами root. Установщик добавляет Docker. Откройте порты 80 и 443. Сначала установите Levelrail одним из способов в разделе [Установка](#установка).

1. Откройте ссылку настройки, которую выводит установщик, и задайте пароль. Так создаётся учётная запись администратора.
2. В панели скопируйте строку входа из карточки **Connect your CLI** и выполните её (она выглядит так: `levelrail-cli login --api-url http://127.0.0.1:8080 --code ABCD-EF23`).
3. Разверните первое приложение:

```bash
levelrail-cli import https://github.com/your-org/your-app --deploy
```

Если что-то пошло не так, запустите `levelrail-cli doctor` и загляните в [Установка](../../docs/installing.md#troubleshooting). Дальше: [Первые 15 минут](../../docs/first-15-minutes.md). Только CLI: `brew install glincker/tap/levelrail-cli` или `npx levelrail-cli --help`.

Хотите сначала посмотреть? [Откройте живую демо](https://demo.levelrail.com), размещённую демо-версию панели только для чтения с вымышленными данными ([как она работает](../../docs/run-a-read-only-demo.md)).

## Где можно использовать

| Поверхность | Старт | Документация |
| --- | --- | --- |
| Веб-панель | Откройте ссылку настройки, которую выводит установщик | [Первые 15 минут](../../docs/first-15-minutes.md) |
| CLI | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/apple/default.svg" alt="Apple" title="Apple" width="20" height="20"> `brew install glincker/tap/levelrail-cli`, `npx levelrail-cli --help` или `npm install --global levelrail-cli` | [Установка](../../docs/installing.md) |
| HTTP API | Bearer-токен, `/api/v1` | [Справочник API](../../docs/api-reference.md) |
| Terraform и OpenTofu | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/terraform/default.svg" alt="Terraform" title="Terraform" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/opentofu/default.svg" alt="OpenTofu" title="OpenTofu" width="20" height="20"> `source = "glincker/levelrail"` | [Провайдер Terraform](../../docs/terraform-provider.md) |
| MCP и кодинг-агенты | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/model-context-protocol/default.svg" alt="MCP" title="MCP" width="20" height="20"> 259 инструментов | [MCP](../../docs/mcp.md) |
| Мобильный API (бета, по умолчанию выключен) | Включите в Настройки, Бета-функции | [Мобильный API](../../docs/mobile-api.md) |
| Desktop-рантайм на Mac (экспериментально) | Частная Linux-ВМ на вашем Mac | [Desktop-рантайм](../../docs/desktop-runtime.md) |

## Что вы получаете

Зрелость соответствует [статусу функций](../../docs/feature-status.md).

| Область | Одной строкой | Работает с |
| --- | --- | --- |
| [Деплой](../../docs/deploying-apps.md) | Git push из GitHub, GitLab, Bitbucket, Gitea или Forgejo, выкатка с проверкой готовности, откат в один шаг, превью и канареечные релизы | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/github/default.svg" alt="GitHub" title="GitHub" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitlab/default.svg" alt="GitLab" title="GitLab" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/bitbucket/default.svg" alt="Bitbucket" title="Bitbucket" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitea/default.svg" alt="Gitea" title="Gitea" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/docker/default.svg" alt="Docker" title="Docker" width="20" height="20"> |
| [Шаблоны](../../docs/template-catalog.md) | 570+ шаблонов в каталоге, у 346 есть записанный прогон на реальном движке | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/postgresql/default.svg" alt="PostgreSQL" title="PostgreSQL" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/redis/default.svg" alt="Redis" title="Redis" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/grafana/default.svg" alt="Grafana" title="Grafana" width="20" height="20"> |
| [Edge и HTTPS](../../docs/domains-and-ingress.md) | Автоматические сертификаты, включая wildcard, файрвол, списки ботов, заголовки и редиректы для каждого домена | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/letsencrypt/default.svg" alt="Let's Encrypt" title="Let's Encrypt" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/cloudflare/default.svg" alt="Cloudflare" title="Cloudflare" width="20" height="20"> |
| [Узлы и облака](../../docs/node-provisioning.md) | Подключение любого Linux-хоста по токену, создание узлов в 6 облаках (бета), существующие кластеры Kubernetes как цели (экспериментально) | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/hetzner/default.svg" alt="Hetzner" title="Hetzner" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/digitalocean/default.svg" alt="DigitalOcean" title="DigitalOcean" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/aws/default.svg" alt="AWS" title="AWS" width="20" height="20"> |
| [Базы данных и резервные копии](../../docs/managing-databases.md) | 13 движков, резервные копии в S3-совместимое хранилище, R2 или на локальный диск, [проверки восстановления](../../docs/prove-your-backups-work.md) | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/postgresql/default.svg" alt="PostgreSQL" title="PostgreSQL" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/mysql/default.svg" alt="MySQL" title="MySQL" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/mariadb/default.svg" alt="MariaDB" title="MariaDB" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/mongodb/default.svg" alt="MongoDB" title="MongoDB" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/redis/default.svg" alt="Redis" title="Redis" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/aws-amazon-simple-storage-service/default.svg" alt="Amazon S3" title="Amazon S3" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/minio/default.svg" alt="MinIO" title="MinIO" width="20" height="20"> |
| [Наблюдаемость](../../docs/observability.md) | Встроенные метрики, поиск по логам, алерты в 18 видов каналов, приём OTLP, страница статуса. Grafana читает логи через [Loki-совместимый API](../../docs/loki-api.md) | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/grafana/default.svg" alt="Grafana" title="Grafana" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/prometheus/default.svg" alt="Prometheus" title="Prometheus" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/slack/default.svg" alt="Slack" title="Slack" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/discord/default.svg" alt="Discord" title="Discord" width="20" height="20"> |
| [Безопасность и доступ](../../docs/identity-and-access.md) | Вход через OAuth и OIDC, passkey, TOTP, политики IAM, аудит-лог с хеш-цепочкой | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/github/default.svg" alt="GitHub" title="GitHub" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitlab/default.svg" alt="GitLab" title="GitLab" width="20" height="20"> |
| [Автоматизация](../../docs/cli-reference.md) | CLI, HTTP API, [провайдер Terraform и OpenTofu](../../docs/terraform-provider.md), 259 [инструментов MCP](../../docs/mcp.md) для кодинг-агентов | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/terraform/default.svg" alt="Terraform" title="Terraform" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/opentofu/default.svg" alt="OpenTofu" title="OpenTofu" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/model-context-protocol/default.svg" alt="MCP" title="MCP" width="20" height="20"> |

<details>
<summary>Все функции деплоя</summary>

- Деплой по Git push или вебхуку из GitHub, GitLab, Bitbucket, Gitea и Forgejo. См. [Интеграции с Git](../../docs/git-integrations.md) и [Деплой по Git push](../../docs/git-push-deploy.md).
- Сборки Dockerfile, Compose и Railpack (Railpack в бете) с очередью сборок. См. [Build Center](../../docs/build-center.md).
- Выкатка с проверкой готовности и откат в один шаг. См. [Деплой без простоя](../../docs/zero-downtime-deploys.md) и [Откат](../../docs/rollback.md).
- Канареечные релизы и сплит-тесты. См. [Канареечные деплои](../../docs/canary-deploys.md) и [Сплит-тестирование](../../docs/split-testing.md).
- Превью-окружения для каждого pull request: стабильно на GitHub, бета на GitLab и Bitbucket. См. [Превью деплоев](../../docs/deploy-previews.md).
- Подтверждения деплоя и заморозки деплоя. См. [Безопасность деплоя](../../docs/deploy-safety.md).
- Функции на Node, Python или Go (бета). См. [Функции](../../docs/functions.md).
</details>
<details>
<summary>Все подробности о шаблонах</summary>

- 574 записи в каталоге (565 активных, 9 устаревших) в 22 категориях.
- У 346 записей есть записанный прогон на реальном движке.
- 228 записей помечены как превью: их можно установить, но записанного прогона пока нет.
</details>
<details>
<summary>Все функции Edge</summary>

- Встроенный ingress с автоматическим TLS через HTTP-01 и wildcard DNS-01.
- Заголовки, форвардеры, гео-правила, кеширование и редиректы для каждого домена. См. [Управление трафиком домена](../../docs/domain-traffic-controls.md).
- Правила файрвола, списки IP, проверка браузера, списки ботов и режим атаки. См. [Файрвол](../../docs/firewall.md).
- Лимиты запросов и списки блокировки для каждого приложения (бета). См. [Защита от злоупотреблений](../../docs/abuse-defense.md).
- Round-robin балансировка между репликами. См. [Балансировка нагрузки](../../docs/load-balancing.md).
- Частные сети и mesh на WireGuard (бета). См. [Несколько узлов](../../docs/multi-node.md).
</details>
<details>
<summary>Все функции узлов и облаков</summary>

- Создание узлов в Hetzner, DigitalOcean, AWS, Azure, GCP или Oracle Cloud (бета).
- Подключение любого Linux-хоста одноразовым токеном по mTLS. См. [Добавьте узел где угодно](../../docs/add-node-anywhere.md).
- DNS в Cloudflare и Route53, реестры образов в ECR, Artifact Registry и ACR. Статус по каждому облаку см. в [Облачных интеграциях](../../docs/cloud-integrations.md).
- Работает с уже имеющимися у вас кластерами и облачными аккаунтами (экспериментально): подключайте существующие кластеры Kubernetes и деплойте в них, по умолчанию выключено. См. [Кластеры Kubernetes](../../docs/kubernetes-clusters.md).
</details>
<details>
<summary>Все функции данных и резервных копий</summary>

- 13 движков баз данных в коде. 9 проверены вживую. 4 (Meilisearch, Qdrant, Typesense, Memcached) реализованы, но ещё не проверены вживую.
- Резервные копии в S3-совместимые хранилища, R2 или локальные цели. См. [Резервные копии и хранилище](../../docs/backups-and-storage.md).
- Проверки восстановления, которые доказывают, что резервная копия работает. См. [Докажите, что ваши резервные копии работают](../../docs/prove-your-backups-work.md).
- Восстановление на момент времени для Postgres.
</details>
<details>
<summary>Все функции наблюдаемости</summary>

- Встроенные метрики, поиск по логам и архив логов. См. [Обозреватель логов](../../docs/log-explorer.md) и [Архив логов](../../docs/log-archive.md).
- Приём OTLP. См. [Приём OTLP](../../docs/otlp-ingest.md).
- Публичная страница статуса. См. [Страница статуса](../../docs/status-page.md).
- Трейсы деплоя и экспорт трафика. См. [Трейс деплоя](../../docs/deploy-trace.md) и [Экспорт трафика](../../docs/traffic-export.md).
- Алерты в 18 видов каналов уведомлений. См. [Алерты](../../docs/alerts.md).
- Мониторы доступности HTTP, TCP, heartbeat и TLS (экспериментально). См. [Мониторы](../../docs/monitors.md).
</details>
<details>
<summary>Все функции безопасности и доступа</summary>

- Пароли, passkey и TOTP.
- Вход через OAuth с 20 провайдерами и универсальный OIDC. См. [OIDC](../../docs/sso-oidc.md).
- SAML и SCIM (бета, по умолчанию выключено). См. [SAML и SCIM](../../docs/sso-saml.md).
- Политики IAM и команды. См. [Команды и роли](../../docs/teams-and-roles.md).
- Аудит-лог с хеш-цепочкой. См. [Аудит-лог](../../docs/audit-log.md).
- Зашифрованные секреты. См. [Окружение и секреты](../../docs/environment-and-secrets.md).
- Подписанные релизы. См. [Проверка релиза](../../docs/verify-a-release.md).
</details>
<details>
<summary>Все функции автоматизации</summary>

- `levelrail-cli` и HTTP API. См. [Справочник API](../../docs/api-reference.md).
- Провайдер Terraform и OpenTofu. См. [Провайдер Terraform](../../docs/terraform-provider.md), [реестр Terraform](https://registry.terraform.io/providers/glincker/levelrail/latest) и [реестр OpenTofu](https://search.opentofu.org/provider/glincker/levelrail/latest).
- Платформа как код в YAML (экспериментально). См. [Платформа как код](../../docs/platform-as-code.md).
- CI-пайплайны. См. [Пайплайны](../../docs/pipelines.md).
- 11 интеграций для каждого приложения: Sentry, PostHog, GlitchTip, OneUptime, Plausible, Datadog, Axiom, Better Stack, LogSnag, Bugsnag и New Relic. См. [Алерты](../../docs/alerts.md).
- Подписанные плагины для каналов уведомлений. См. [Плагины](../../docs/plugins.md).
- 259 инструментов MCP для кодинг-агентов. См. [MCP](../../docs/mcp.md) и [Набор инструментов](../../docs/mcp-tool-surface.md).
- Шаблон почтового сервера с ящиками и DNS-записями (экспериментально). См. [Почтовый шаблон](../../docs/mail-template.md).
</details>

Все функции с метками зрелости: [docs/features.md](../../docs/features.md).

## Создан для кодинг-агентов

259 [инструментов MCP](../../docs/mcp.md), ограниченных правами токена, с подтверждением записи и аудитом. Добавьте навык командой `npx skills add https://github.com/glincker/levelrail --skill levelrail`, затем попросите агента задеплоить ваш репозиторий. Чтобы установить через агента, вставьте это в Claude Code, Cursor или любой агент, который умеет выполнять команды оболочки на вашем сервере:

```text
Install Levelrail on this server: run `sudo sh install.sh` (download and verify it first, see [GitHub release](https://github.com/glincker/levelrail/releases/latest)).
Open the setup link it prints and tell me, because I choose the password myself.
Then run `levelrail-cli doctor` and `levelrail-cli status` and report anything that fails.
Do not write any token or password to a file or into this chat.
```

## Готов к продакшену, с доказательствами

| Утверждение | Доказательство |
| --- | --- |
| Каждая функция помечена как стабильная или бета | [Статус функций](../../docs/feature-status.md) |
| Переживает сбои | [Отказоустойчивость](../../docs/resilience.md) |
| Измерено, а не обещано | [Бенчмарки](../../docs/benchmarks.md) |
| Проверено на реальных контейнерах | [CI](../../.github/workflows/ci.yml) и [`test/e2e`](../../test/e2e) |
| Подписанные релизы | [Проверьте их](../../docs/installing.md#verifying-release-binaries) |
| Честно о пробелах | [Известные ограничения](../../docs/known-limits.md) |

## Чем Levelrail выделяется

Каждое утверждение ведёт на страницу, которая его подтверждает. Столбец статуса соответствует [статусу функций](../../docs/feature-status.md).

| Что вы получаете | Статус | Доказательство |
| --- | --- | --- |
| Плоскость управления, которая в простое занимает около 200 МБ ОЗУ, в одном Go-бинарнике | измерено | [Производительность](../../docs/performance.md), [Бенчмарки](../../docs/benchmarks.md) |
| `git push` на ваш сервер по HTTPS, без провайдера, вебхука и SSH-ключа (деплой по вебхуку GitHub стабилен) | бета | [Деплой через git push](../../docs/git-push-deploy.md) |
| Откат на зафиксированный образ, выкатка с проверкой готовности и автоматический откат по наблюдению после деплоя | стабильно (сигнал задержки в бете) | [Безопасность деплоя](../../docs/deploy-safety.md) |
| Проверяемые резервные копии с проверками восстановления в тестовый ресурс | Postgres стабильно, остальные движки в бете | [Докажите, что ваши резервные копии работают](../../docs/prove-your-backups-work.md), [Резервные копии и хранилище](../../docs/backups-and-storage.md) |
| Аудит-лог с хеш-цепочкой, который можно проверять, фильтровать и экспортировать | включён по умолчанию | [Аудит-лог](../../docs/audit-log.md) |
| Агент узла, который сам подключается к плоскости управления по mTLS, поэтому управляемым узлам не нужны входящие порты | несколько узлов в бете | [Архитектура](../../docs/architecture.md), [Несколько узлов](../../docs/multi-node.md) |
| Сервер MCP с инструментами, ограниченными правами, и записью с подтверждением, никогда не в пути reconcile | бета | [MCP](../../docs/mcp.md), [Набор инструментов](../../docs/mcp-tool-surface.md) |

Переходите с другой платформы? См. [Переход на Levelrail](../../docs/migrate-to-levelrail.md). Заметки об архитектуре: [Сравнение](../../docs/comparison.md).

## Сообщество

[Обсуждения](https://github.com/glincker/levelrail/discussions) &middot; [Issues](https://github.com/glincker/levelrail/issues) &middot; [Discord](https://discord.gg/Ar5pcaZB99) &middot; [support@levelrail.com](mailto:support@levelrail.com) &middot; [Политика безопасности](../../.github/SECURITY.md)

Если Levelrail сэкономит вам выходные, [звезда](https://github.com/glincker/levelrail) поможет другим самохостерам его найти.

## Лицензия

Apache 2.0, см. [LICENSE](../../LICENSE). Автор: Gagan Singh aka GDS K S из [GLINR Studios](https://glinr.com) и [theSVG](https://thesvg.org).

