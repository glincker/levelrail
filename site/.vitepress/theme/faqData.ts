export interface FaqItem {
  q: string
  a: string
}

// Shared by FaqSection.vue (renders it) and config.mts (FAQPage JSON-LD
// on the homepage) so the two cannot drift. Check every count and claim
// against the code before editing, and keep it in line with comparison.md.
export const faqItems: FaqItem[] = [
  {
    q: 'What is Levelrail?',
    a: 'A self-hosted platform that turns one or more Linux servers into a private cloud. Point it at a git repository or a Docker image and it builds, deploys, and keeps the app running, with TLS, logs, metrics, and rollback. The control plane and the node agent are each a single Go binary, and Docker is the only container runtime.',
  },
  {
    q: 'How is this different from Coolify or Railway?',
    a: "Most self-hosted platforms in this category, Coolify included, manage remote servers by SSHing in and running docker CLI commands, then parsing the text output. Levelrail's agent dials out over mTLS and talks to the Docker Engine API directly, nothing shells out to the docker CLI, and a level-triggered reconciler replaces polling loops. Railway is a managed cloud you don't control, while Levelrail runs on servers you own. The comparison page covers where the other projects are the better choice.",
  },
  {
    q: 'Do I need to know Kubernetes?',
    a: 'No. Levelrail is not a Kubernetes alternative: there is no scheduler with bin-packing or autoscaling, no CRDs, and nothing to learn beyond apps, domains, and databases. It is built for running 3 to 50 services on 1 to 10 machines without operating a cluster.',
  },
  {
    q: 'Is it free?',
    a: 'Yes. Levelrail is Apache 2.0 licensed with one edition: no license key, no enterprise tier, and no feature gated behind a plan. OAuth sign-in, two-factor authentication, audit logging, resource-scoped IAM, and scheduled backups all ship in the same binary for everyone. SAML and SCIM are not supported yet.',
  },
  {
    q: 'Is it ready for production?',
    a: 'Not yet. There is no stable release, and APIs, the app spec format, and the on-disk layout can change between betas. A single node is the well-tested path, and the feature status page labels every area as stable, beta, or behind a flag, with the evidence for each.',
  },
  {
    q: 'Can I run it on a single server?',
    a: 'Yes, and that is the default. On one server the control plane runs the node agent in the same process and talks to it through an in-process transport, so there is nothing extra to install. The same code path serves one node or ten.',
  },
  {
    q: 'What happens if a deploy fails?',
    a: 'With the default blue-green strategy, and with rolling, traffic only moves to the new container once its readiness probe passes, not once the process merely starts. If the probe never passes, the old container keeps serving and the deploy fails with a specific reason such as OOMKilledDuringReadiness. The recreate strategy stops the old container before starting the new one, so expect a short gap. Prior images stay pinned, so garbage collection cannot remove a rollback target.',
  },
  {
    q: 'Does it support multiple servers?',
    a: 'Yes, and it is in beta. A second server joins with a one-time token and gets its own client certificate. Every agent dials out to the control plane, so managed servers need no inbound ports. You can cordon and drain nodes and pin apps to them. The WireGuard mesh and cross-host service DNS are the least verified part of this area.',
  },
  {
    q: 'Is there an AI or MCP integration?',
    a: 'Yes, in beta. An MCP server exposes 156 tools over the same HTTP API and permission model the dashboard uses, so an AI client can list apps, read logs, get metrics, diagnose a failed deploy, trigger a deploy, or roll back. A token limited to fewer abilities gets the same 403 the REST API returns. AI is never part of the reconciliation path.',
  },
]
