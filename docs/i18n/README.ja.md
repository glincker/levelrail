<!-- translated-from: README.md blob:b3c2e12bc506fb9fd5a89df18863898321998952 lang:ja -->
<div align="center">
<img src="../../docs/assets/brand/levelrail-default.svg" alt="Levelrail ロゴ" width="56" height="56">
<h1>Levelrail</h1>
<p><strong>自分のサーバーへ、git push で本番デプロイ。</strong><br>
Go 製の単一バイナリで動くセルフホスト型デプロイプラットフォームです。コントロールプレーンはアイドル時に約 200 MB の RAM を使い（<a href="../../docs/performance.md">計測値</a>）、570+ のワンクリックアプリ、ロールバック、プレビュー、HTTPS、データベースを標準で備えています。</p>
<p><code>sudo sh install.sh</code> (<a href="https://github.com/glincker/levelrail/releases/latest">GitHub release</a>) &middot; <a href="https://demo.levelrail.com"><b>ライブデモを試す</b></a></p>
<p>
  <a href="https://github.com/glincker/levelrail/stargazers"><img src="https://img.shields.io/github/stars/glincker/levelrail?style=flat&logo=github" alt="GitHub スター数"></a>
  <a href="../../LICENSE"><img src="https://img.shields.io/github/license/glincker/levelrail" alt="ライセンス：Apache 2.0"></a>
  <a href="https://github.com/glincker/levelrail/releases"><img src="https://img.shields.io/github/v/release/glincker/levelrail?include_prereleases&label=release" alt="最新リリース"></a>
  <a href="https://github.com/glincker/levelrail/actions/workflows/ci.yml"><img src="https://github.com/glincker/levelrail/actions/workflows/ci.yml/badge.svg" alt="CI の状態"></a>
  <a href="https://demo.levelrail.com"><img src="https://img.shields.io/badge/demo-demo.levelrail.com-2f6feb" alt="ライブデモ"></a>
</p>
<p>
  <a href="https://levelrail.com?utm_source=github&amp;utm_medium=readme&amp;utm_campaign=nav">Web サイト</a> &middot;
  <a href="https://docs.levelrail.com?utm_source=github&amp;utm_medium=readme&amp;utm_campaign=nav">ドキュメント</a> &middot;
  <a href="#クイックスタート">クイックスタート</a> &middot;
  <a href="#levelrail-の特長">比較</a> &middot;
  <a href="../../docs/faq.md">FAQ</a> &middot;
  <a href="../../docs/roadmap.md">ロードマップ</a> &middot;
  <a href="https://github.com/glincker/levelrail/discussions">ディスカッション</a>
</p>
<!-- i18n-nav:start -->
<p><a href="../../README.md">English</a> &middot; <a href="README.zh-Hans.md">简体中文</a> &middot; <a href="README.es.md">Español</a> &middot; <a href="README.pt-BR.md">Português (BR)</a> &middot; <a href="README.de.md">Deutsch</a> &middot; <a href="README.fr.md">Français</a> &middot; 日本語</p>
<!-- i18n-nav:end -->
<img src="../../docs/assets/demos/product-tour.gif" alt="Levelrail ダッシュボードのツアー：フリートの概要、アプリ一覧、デプロイ履歴、組み込みメトリクス" width="880">
</div>

> この翻訳は AI によって生成されており、誤りが含まれている可能性があります。修正を歓迎します：[docs/i18n/README.md](README.md) を参照してください。

v1.x。単一サーバーは本番運用可能で、マルチノードはベータです。Levelrail は Linux サーバーをプライベートクラウドに変え、すでにお持ちの Kubernetes クラスターやクラウドアカウントとも連携します。すべての機能には、[機能ステータス](../../docs/feature-status.md)で stable、beta、experimental のいずれかのラベルが付いています。

## クイックスタート

systemd と root 権限を持つ Linux サーバー（amd64 または arm64）が必要です。インストーラーが Docker を追加します。ポート 80 と 443 を開けてください。

```bash
curl -fsSLO https://github.com/glincker/levelrail/releases/latest/download/install.sh
curl -fsSLO https://github.com/glincker/levelrail/releases/latest/download/checksums.txt
grep ' install.sh$' checksums.txt | sha256sum -c -  # macOS: shasum -a 256 -c -
sudo sh install.sh
```

1. インストーラーが表示するセットアップリンクを開き、パスワードを設定します。これで管理者アカウントが作成されます。
2. ダッシュボードの **CLI を接続する** カードからログイン用の行をコピーして実行します（`levelrail-cli login --api-url http://127.0.0.1:8080 --code ABCD-EF23` のような形です）。
3. 最初のアプリをデプロイします。

```bash
levelrail-cli import https://github.com/your-org/your-app --deploy
```

うまくいかない場合は `levelrail-cli doctor` を実行し、[インストール](../../docs/installing.md#troubleshooting)を参照してください。次に読むもの：[最初の 15 分](../../docs/first-15-minutes.md)。CLI だけを使う場合：`brew install glincker/tap/levelrail-cli` または `npx levelrail-cli --help`。

まず見てみたい場合は、[ライブデモを試す](https://demo.levelrail.com)をご利用ください。架空のデータを使った、ダッシュボードの読み取り専用のホスト型デモです（[動作の仕組み](../../docs/run-a-read-only-demo.md)）。

## 利用できる場所

| 利用面 | 開始方法 | ドキュメント |
| --- | --- | --- |
| Web ダッシュボード | インストーラーが表示するセットアップリンクを開く | [最初の 15 分](../../docs/first-15-minutes.md) |
| CLI | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/apple/default.svg" alt="Apple" title="Apple" width="20" height="20"> `brew install glincker/tap/levelrail-cli`、`npx levelrail-cli --help` または `npm install --global levelrail-cli` | [インストール](../../docs/installing.md) |
| HTTP API | Bearer トークン、`/api/v1` | [API リファレンス](../../docs/api-reference.md) |
| Terraform と OpenTofu | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/terraform/default.svg" alt="Terraform" title="Terraform" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/opentofu/default.svg" alt="OpenTofu" title="OpenTofu" width="20" height="20"> `source = "glincker/levelrail"` | [Terraform プロバイダー](../../docs/terraform-provider.md) |
| MCP とコーディングエージェント | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/model-context-protocol/default.svg" alt="MCP" title="MCP" width="20" height="20"> 259 ツール | [MCP](../../docs/mcp.md) |
| モバイル API（ベータ、デフォルトで無効） | 設定の「ベータ機能」で有効化 | [モバイル API](../../docs/mobile-api.md) |
| Mac 上のデスクトップランタイム（experimental） | Mac 上のプライベート Linux VM | [デスクトップランタイム](../../docs/desktop-runtime.md) |

## 主な機能

成熟度は[機能ステータス](../../docs/feature-status.md)に従います。

| 分野 | 一言で | 連携先 |
| --- | --- | --- |
| [デプロイ](../../docs/deploying-apps.md) | GitHub、GitLab、Bitbucket、Gitea、Forgejo からの Git push、準備完了ゲート付きのロールアウト、ワンステップのロールバック、プレビュー、カナリア | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/github/default.svg" alt="GitHub" title="GitHub" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitlab/default.svg" alt="GitLab" title="GitLab" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/bitbucket/default.svg" alt="Bitbucket" title="Bitbucket" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitea/default.svg" alt="Gitea" title="Gitea" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/docker/default.svg" alt="Docker" title="Docker" width="20" height="20"> |
| [テンプレート](../../docs/template-catalog.md) | カタログに 570+ のテンプレート、346 件は実エンジンでの実行記録あり | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/postgresql/default.svg" alt="PostgreSQL" title="PostgreSQL" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/redis/default.svg" alt="Redis" title="Redis" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/grafana/default.svg" alt="Grafana" title="Grafana" width="20" height="20"> |
| [エッジと HTTPS](../../docs/domains-and-ingress.md) | ワイルドカードを含む自動証明書、ファイアウォール、ボットリスト、ドメインごとのヘッダーとリダイレクト | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/letsencrypt/default.svg" alt="Let's Encrypt" title="Let's Encrypt" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/cloudflare/default.svg" alt="Cloudflare" title="Cloudflare" width="20" height="20"> |
| [ノードとクラウド](../../docs/node-provisioning.md) | トークンで任意の Linux ホストを参加させる、6 つのクラウドでノードをプロビジョニング（ベータ）、既存の Kubernetes クラスターをターゲットにする（experimental） | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/hetzner/default.svg" alt="Hetzner" title="Hetzner" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/digitalocean/default.svg" alt="DigitalOcean" title="DigitalOcean" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/aws/default.svg" alt="AWS" title="AWS" width="20" height="20"> |
| [データベースとバックアップ](../../docs/managing-databases.md) | 13 のエンジン、S3 互換、R2、ローカルディスクへのバックアップ、[リストアの訓練](../../docs/prove-your-backups-work.md) | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/postgresql/default.svg" alt="PostgreSQL" title="PostgreSQL" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/mysql/default.svg" alt="MySQL" title="MySQL" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/mariadb/default.svg" alt="MariaDB" title="MariaDB" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/mongodb/default.svg" alt="MongoDB" title="MongoDB" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/redis/default.svg" alt="Redis" title="Redis" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/aws-amazon-simple-storage-service/default.svg" alt="Amazon S3" title="Amazon S3" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/minio/default.svg" alt="MinIO" title="MinIO" width="20" height="20"> |
| [オブザーバビリティ](../../docs/observability.md) | 組み込みのメトリクス、ログ検索、18 種類のチャンネルへのアラート、OTLP 取り込み、ステータスページ。Grafana は [Loki 互換 API](../../docs/loki-api.md) 経由でログを読み取ります | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/grafana/default.svg" alt="Grafana" title="Grafana" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/prometheus/default.svg" alt="Prometheus" title="Prometheus" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/slack/default.svg" alt="Slack" title="Slack" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/discord/default.svg" alt="Discord" title="Discord" width="20" height="20"> |
| [セキュリティとアクセス](../../docs/identity-and-access.md) | OAuth と OIDC によるサインイン、パスキー、TOTP、IAM ポリシー、ハッシュチェーン式の監査ログ | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/github/default.svg" alt="GitHub" title="GitHub" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitlab/default.svg" alt="GitLab" title="GitLab" width="20" height="20"> |
| [自動化](../../docs/cli-reference.md) | CLI、HTTP API、[Terraform と OpenTofu のプロバイダー](../../docs/terraform-provider.md)、コーディングエージェント向けの 259 の [MCP ツール](../../docs/mcp.md) | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/terraform/default.svg" alt="Terraform" title="Terraform" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/opentofu/default.svg" alt="OpenTofu" title="OpenTofu" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/model-context-protocol/default.svg" alt="MCP" title="MCP" width="20" height="20"> |

<details>
<summary>デプロイ機能の一覧</summary>

- GitHub、GitLab、Bitbucket、Gitea、Forgejo からの Git push または Webhook によるデプロイ。[Git 連携](../../docs/git-integrations.md)と [Git push デプロイ](../../docs/git-push-deploy.md)を参照してください。
- ビルドキュー付きの Dockerfile、Compose、Railpack ビルド（Railpack はベータ）。[Build Center](../../docs/build-center.md)を参照してください。
- 準備完了ゲート付きのロールアウトと、ワンステップのロールバック。[ゼロダウンタイムデプロイ](../../docs/zero-downtime-deploys.md)と[ロールバック](../../docs/rollback.md)を参照してください。
- カナリアとスプリットテスト。[カナリアデプロイ](../../docs/canary-deploys.md)と[スプリットテスト](../../docs/split-testing.md)を参照してください。
- プルリクエストごとのプレビュー環境：GitHub では stable、GitLab と Bitbucket ではベータ。[デプロイプレビュー](../../docs/deploy-previews.md)を参照してください。
- デプロイの承認とデプロイのフリーズ。[デプロイの安全性](../../docs/deploy-safety.md)を参照してください。
- Node、Python、Go の関数（ベータ）。[関数](../../docs/functions.md)を参照してください。
</details>
<details>
<summary>テンプレートの詳細</summary>

- 22 カテゴリに 574 件のカタログエントリ（565 件が有効、9 件が非推奨）。
- 346 件のエントリに、実エンジンでの実行記録があります。
- 228 件のエントリはプレビューとしてマークされています：インストールは可能ですが、実行記録はまだありません。
</details>
<details>
<summary>エッジ機能の一覧</summary>

- HTTP-01 と DNS-01 ワイルドカードによる自動 TLS を備えた組み込みイングレス。
- ドメインごとのヘッダー、フォワーダー、地域ルール、キャッシュ、リダイレクト。[ドメインのトラフィック制御](../../docs/domain-traffic-controls.md)を参照してください。
- ファイアウォールルール、IP リスト、ブラウザーチャレンジ、ボットリスト、アタックモード。[ファイアウォール](../../docs/firewall.md)を参照してください。
- アプリごとのレート制限と拒否リスト（ベータ）。[不正利用対策](../../docs/abuse-defense.md)を参照してください。
- レプリカ間のラウンドロビン負荷分散。[ロードバランシング](../../docs/load-balancing.md)を参照してください。
- プライベートネットワークと WireGuard メッシュ（ベータ）。[マルチノード](../../docs/multi-node.md)を参照してください。
</details>
<details>
<summary>ノードとクラウド機能の一覧</summary>

- Hetzner、DigitalOcean、AWS、Azure、GCP、Oracle Cloud でのノードのプロビジョニング（ベータ）。
- mTLS 経由で、1 回限りのトークンを使って任意の Linux ホストを参加させます。[どこにでもノードを追加](../../docs/add-node-anywhere.md)を参照してください。
- Cloudflare と Route53 の DNS、ECR、Artifact Registry、ACR のイメージレジストリ。クラウドごとのステータスは[クラウド連携](../../docs/cloud-integrations.md)を参照してください。
- すでにお持ちのクラスターやクラウドアカウントと連携（experimental）：既存の Kubernetes クラスターを接続してデプロイできます。デフォルトでは無効です。[Kubernetes クラスター](../../docs/kubernetes-clusters.md)を参照してください。
</details>
<details>
<summary>データとバックアップ機能の一覧</summary>

- コード上で 13 のデータベースエンジンに対応。9 件は実環境で検証済み。4 件（Meilisearch、Qdrant、Typesense、Memcached）は実装済みですが、まだ実環境で検証されていません。
- S3 互換、R2、ローカルのターゲットへのバックアップ。[バックアップとストレージ](../../docs/backups-and-storage.md)を参照してください。
- バックアップが機能することを証明するリストアの訓練。[バックアップが機能することを証明する](../../docs/prove-your-backups-work.md)を参照してください。
- Postgres のポイントインタイムリカバリ。
</details>
<details>
<summary>オブザーバビリティ機能の一覧</summary>

- 組み込みのメトリクス、ログ検索、ログアーカイブ。[ログエクスプローラー](../../docs/log-explorer.md)と[ログアーカイブ](../../docs/log-archive.md)を参照してください。
- OTLP 取り込み。[OTLP 取り込み](../../docs/otlp-ingest.md)を参照してください。
- 公開ステータスページ。[ステータスページ](../../docs/status-page.md)を参照してください。
- デプロイのトレースとトラフィックのエクスポート。[デプロイトレース](../../docs/deploy-trace.md)と[トラフィックのエクスポート](../../docs/traffic-export.md)を参照してください。
- 18 種類の通知チャンネルへのアラート。[アラート](../../docs/alerts.md)を参照してください。
- HTTP、TCP、ハートビート、TLS の稼働監視（experimental）。[モニター](../../docs/monitors.md)を参照してください。
</details>
<details>
<summary>セキュリティとアクセス機能の一覧</summary>

- パスワード、パスキー、TOTP。
- 20 プロバイダーの OAuth サインインと汎用 OIDC。[OIDC](../../docs/sso-oidc.md)を参照してください。
- SAML と SCIM（ベータ、デフォルトで無効）。[SAML と SCIM](../../docs/sso-saml.md)を参照してください。
- IAM ポリシーとチーム。[チームとロール](../../docs/teams-and-roles.md)を参照してください。
- ハッシュチェーン式の監査ログ。[監査ログ](../../docs/audit-log.md)を参照してください。
- 暗号化されたシークレット。[環境とシークレット](../../docs/environment-and-secrets.md)を参照してください。
- 署名付きリリース。[リリースを検証する](../../docs/verify-a-release.md)を参照してください。
</details>
<details>
<summary>自動化機能の一覧</summary>

- `levelrail-cli` と HTTP API。[API リファレンス](../../docs/api-reference.md)を参照してください。
- Terraform と OpenTofu のプロバイダー。[Terraform プロバイダー](../../docs/terraform-provider.md)、[Terraform レジストリ](https://registry.terraform.io/providers/glincker/levelrail/latest)、[OpenTofu レジストリ](https://search.opentofu.org/provider/glincker/levelrail/latest)を参照してください。
- YAML によるプラットフォームのコード化（experimental）。[プラットフォームのコード化](../../docs/platform-as-code.md)を参照してください。
- CI パイプライン。[パイプライン](../../docs/pipelines.md)を参照してください。
- アプリごとの 11 の連携：Sentry、PostHog、GlitchTip、OneUptime、Plausible、Datadog、Axiom、Better Stack、LogSnag、Bugsnag、New Relic。[アラート](../../docs/alerts.md)を参照してください。
- 通知チャンネル用の署名付きプラグイン。[プラグイン](../../docs/plugins.md)を参照してください。
- コーディングエージェント向けの 259 の MCP ツール。[MCP](../../docs/mcp.md)と[ツールの範囲](../../docs/mcp-tool-surface.md)を参照してください。
- メールボックスと DNS レコード付きのメールサーバーテンプレート（experimental）。[メールテンプレート](../../docs/mail-template.md)を参照してください。
</details>

成熟度ラベル付きのすべての機能：[docs/features.md](../../docs/features.md)。

## コーディングエージェント向けに構築

259 の [MCP ツール](../../docs/mcp.md)は、トークンの権限にスコープが限定され、書き込みは確認が必須で、監査されます。`npx skills add https://github.com/glincker/levelrail --skill levelrail` でスキルを追加し、エージェントにリポジトリのデプロイを頼んでください。エージェントでインストールするには、サーバー上でシェルコマンドを実行できる Claude Code、Cursor などのエージェントに、次の内容を貼り付けます。

```text
Install Levelrail on this server: run `sudo sh install.sh` (download and verify it first, see [GitHub release](https://github.com/glincker/levelrail/releases/latest)).
Open the setup link it prints and tell me, because I choose the password myself.
Then run `levelrail-cli doctor` and `levelrail-cli status` and report anything that fails.
Do not write any token or password to a file or into this chat.
```

## 本番運用可能と根拠

| 主張 | 根拠 |
| --- | --- |
| すべての機能に stable または beta のラベルがある | [機能ステータス](../../docs/feature-status.md) |
| 障害に耐える | [レジリエンス](../../docs/resilience.md) |
| 約束ではなく計測 | [ベンチマーク](../../docs/benchmarks.md) |
| 実際のコンテナでテスト済み | [CI](../../.github/workflows/ci.yml)と [`test/e2e`](../../test/e2e) |
| 署名付きリリース | [検証する](../../docs/installing.md#verifying-release-binaries) |
| 不足点も正直に | [既知の制限](../../docs/known-limits.md) |

## Levelrail の特長

各項目は、それを裏付けるページにリンクしています。ステータス列は[機能ステータス](../../docs/feature-status.md)に従います。

| 得られるもの | ステータス | 根拠 |
| --- | --- | --- |
| 単一の Go バイナリで、アイドル時に約 200 MB の RAM で動くコントロールプレーン | 計測済み | [パフォーマンス](../../docs/performance.md)、[ベンチマーク](../../docs/benchmarks.md) |
| プロバイダー、Webhook、SSH キーなしで、自分のサーバーへ HTTPS で `git push`（GitHub Webhook デプロイは stable） | beta | [git push でデプロイ](../../docs/git-push-deploy.md) |
| ピン留めイメージへのロールバック、準備完了ゲート付きのロールアウト、デプロイ後ウォッチによる自動ロールバック | stable（レイテンシ信号は beta） | [デプロイの安全性](../../docs/deploy-safety.md) |
| 検証済みバックアップと、スクラッチリソースへのリストアドリル | Postgres は stable、その他のエンジンは beta | [バックアップの検証](../../docs/prove-your-backups-work.md)、[バックアップとストレージ](../../docs/backups-and-storage.md) |
| 検証、絞り込み、エクスポートができるハッシュチェーン付き監査ログ | デフォルトで有効 | [監査ログ](../../docs/audit-log.md) |
| mTLS でコントロールプレーンへ接続するノードエージェント。管理対象ノードに受信ポートは不要 | マルチノードは beta | [アーキテクチャ](../../docs/architecture.md)、[マルチノード](../../docs/multi-node.md) |
| 権限でスコープされたツールと、確認が必要な書き込みを備えた MCP サーバー。リコンサイルの経路には入りません | beta | [MCP](../../docs/mcp.md)、[ツール一覧](../../docs/mcp-tool-surface.md) |

他のプラットフォームからの移行は[Levelrail への移行](../../docs/migrate-to-levelrail.md)を参照してください。アーキテクチャの補足は[比較](../../docs/comparison.md)にあります。

## コミュニティ

[Discussions](https://github.com/glincker/levelrail/discussions) &middot; [Issues](https://github.com/glincker/levelrail/issues) &middot; [Discord](https://discord.gg/Ar5pcaZB99) &middot; [support@levelrail.com](mailto:support@levelrail.com) &middot; [セキュリティポリシー](../../.github/SECURITY.md)

Levelrail があなたの週末を節約できたなら、[スター](https://github.com/glincker/levelrail)を付けていただけると、ほかのセルフホストユーザーが見つけやすくなります。

## ライセンス

Apache 2.0。[LICENSE](../../LICENSE) を参照してください。作者は Gagan Singh aka GDS K S（[GLINR Studios](https://glinr.com) と [theSVG](https://thesvg.org)）です。

<div align="center">


</div>
