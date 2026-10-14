---
title: "Levelrail integrations: backups, registries, providers and sign-in"
sidebarTitle: "Overview"
description: "Every backup destination, registry, cloud provider, sign-in method, database engine, notification channel and git host Levelrail supports, with exact setup steps."
---

Each page below describes one integration: the dashboard path, the CLI command, the permissions it needs and how it fails. Everything listed is implemented today; planned work is called out on the page that would own it.

## [Backup destinations](/integrations/backup-destinations)

S3 compatible buckets that receive database dumps and app volume archives.

[Cloudflare R2](/integrations/backup-destinations/r2), [Backblaze B2](/integrations/backup-destinations/b2), [Wasabi](/integrations/backup-destinations/wasabi), [Amazon S3](/integrations/backup-destinations/aws), [Google Cloud Storage](/integrations/backup-destinations/gcs), [DigitalOcean Spaces](/integrations/backup-destinations/spaces), [Hetzner Object Storage](/integrations/backup-destinations/hetzner), [Scaleway Object Storage](/integrations/backup-destinations/scaleway), [OVHcloud Object Storage](/integrations/backup-destinations/ovh), [Vultr Object Storage](/integrations/backup-destinations/vultr), [MinIO](/integrations/backup-destinations/minio), [Local MinIO (same machine)](/integrations/backup-destinations/local)

## [Container registries](/integrations/container-registries)

Registries Levelrail pulls private images from, with the credential each vendor needs.

[Docker Hub](/integrations/container-registries/dockerhub), [GitHub Container Registry](/integrations/container-registries/ghcr), [GitLab Container Registry](/integrations/container-registries/gitlab), [AWS ECR](/integrations/container-registries/ecr), [Google Artifact Registry](/integrations/container-registries/gar), [Azure Container Registry](/integrations/container-registries/acr), [Harbor](/integrations/container-registries/harbor), [Quay](/integrations/container-registries/quay), [JFrog Artifactory](/integrations/container-registries/jfrog), [Sonatype Nexus](/integrations/container-registries/nexus), [Docker v2 registry](/integrations/container-registries/generic)

## [Package registries](/integrations/package-registries)

Private package sources for Dockerfile builds, mounted as BuildKit secrets.

[npm](/integrations/package-registries/npm), [PyPI](/integrations/package-registries/pypi), [Maven](/integrations/package-registries/maven), [NuGet](/integrations/package-registries/nuget), [Cargo](/integrations/package-registries/cargo), [Go modules](/integrations/package-registries/go), [RubyGems](/integrations/package-registries/rubygems)

## [Cloud node providers](/integrations/cloud-providers)

Providers Levelrail can create servers at and enroll as nodes, plus custom data centers.

[Hetzner Cloud](/integrations/cloud-providers/hetzner), [DigitalOcean](/integrations/cloud-providers/digitalocean), [AWS EC2](/integrations/cloud-providers/aws), [Microsoft Azure](/integrations/cloud-providers/azure), [Google Cloud](/integrations/cloud-providers/gcp), [Custom data center](/integrations/cloud-providers/custom)

## [Sign-in providers](/integrations/sign-in)

OAuth, OpenID Connect, SAML and SCIM options for dashboard access.

[Google](/integrations/sign-in/google), [GitHub](/integrations/sign-in/github), [Microsoft Entra ID](/integrations/sign-in/microsoft), [GitLab](/integrations/sign-in/gitlab), [Bitbucket](/integrations/sign-in/bitbucket), [Discord](/integrations/sign-in/discord), [Slack](/integrations/sign-in/slack), [LinkedIn](/integrations/sign-in/linkedin), [Twitch](/integrations/sign-in/twitch), [Facebook](/integrations/sign-in/facebook), [Linear](/integrations/sign-in/linear), [Vercel](/integrations/sign-in/vercel), [Hugging Face](/integrations/sign-in/huggingface), [Figma](/integrations/sign-in/figma), [Salesforce](/integrations/sign-in/salesforce), [Codeberg](/integrations/sign-in/codeberg), [Amazon Cognito](/integrations/sign-in/cognito), [Generic OpenID Connect](/integrations/sign-in/oidc), [SAML 2.0](/integrations/sign-in/saml), [SCIM 2.0](/integrations/sign-in/scim)

## [Databases](/integrations/databases)

Engines Levelrail runs and backs up as managed databases.

[PostgreSQL](/integrations/databases/postgres), [MySQL](/integrations/databases/mysql), [MariaDB](/integrations/databases/mariadb), [MongoDB](/integrations/databases/mongodb), [Redis](/integrations/databases/redis), [KeyDB](/integrations/databases/keydb), [Dragonfly](/integrations/databases/dragonfly), [ClickHouse](/integrations/databases/clickhouse)

## [Notification channels](/integrations/notifications)

Where alert rules and deploy events can be delivered.

[Generic webhook](/integrations/notifications/generic), [Slack](/integrations/notifications/slack), [Discord](/integrations/notifications/discord), [Telegram](/integrations/notifications/telegram), [Email](/integrations/notifications/email), [Pushover](/integrations/notifications/pushover), [PagerDuty](/integrations/notifications/pagerduty), [Microsoft Teams](/integrations/notifications/teams), [Resend](/integrations/notifications/resend), [ntfy](/integrations/notifications/ntfy), [Gotify](/integrations/notifications/gotify), [Mattermost](/integrations/notifications/mattermost), [Lark](/integrations/notifications/lark), [Rocket.Chat](/integrations/notifications/rocketchat), [Opsgenie](/integrations/notifications/opsgenie), [Webex](/integrations/notifications/webex), [Google Chat](/integrations/notifications/googlechat), [Browser push](/integrations/notifications/webpush)

## [Git providers](/integrations/git-providers)

Hosts Levelrail connects to for push deploys and pull request previews.

[GitHub](/integrations/git-providers/github), [GitLab](/integrations/git-providers/gitlab), [Bitbucket Cloud](/integrations/git-providers/bitbucket), [Gitea](/integrations/git-providers/gitea)

## [App integrations](/integrations/app-integrations)

Curated monitoring and analytics tools whose environment variables are injected into an app.

[Sentry](/integrations/app-integrations/sentry), [GlitchTip](/integrations/app-integrations/glitchtip), [PostHog](/integrations/app-integrations/posthog), [OneUptime](/integrations/app-integrations/oneuptime), [Plausible](/integrations/app-integrations/plausible), [Datadog](/integrations/app-integrations/datadog), [Axiom](/integrations/app-integrations/axiom), [Better Stack](/integrations/app-integrations/betterstack), [LogSnag](/integrations/app-integrations/logsnag), [BugSnag](/integrations/app-integrations/bugsnag), [New Relic](/integrations/app-integrations/newrelic)

