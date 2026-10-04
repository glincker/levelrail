export interface FaqItem {
  q: string
  a: string
}

// Content grounded in CLAUDE.md sections 1, 2, 4 and 6, and comparison.md.
// No invented features: every answer maps to a locked architecture
// decision or a shipped/phased item, not aspirational copy.
// Shared by FaqSection.vue (renders it) and config.mts (FAQPage JSON-LD
// on the homepage) so the two can never drift out of sync.
export const faqItems: FaqItem[] = [
  {
    q: 'What is Levelrail?',
    a: 'A self-hosted platform that turns one or more Linux boxes into a private cloud. Push to a git repo and get a running app with TLS, logs, metrics, and rollback. The control plane and the node agent are each a single Go binary, and Docker is the only container runtime.',
  },
  {
    q: 'How is this different from Coolify or Railway?',
    a: "Most self-hosted platforms in this category, Coolify included, drive remote servers by SSHing in and shelling out docker CLI commands, then parsing text output. Levelrail's agent dials out over mTLS and talks to the Docker Engine API directly, nothing shells out to the docker CLI, and a level-triggered reconciler replaces ad hoc polling loops. Railway is a managed cloud you don't control; Levelrail runs on servers you own.",
  },
  {
    q: 'Do I need to know Kubernetes?',
    a: "No. Levelrail is not a Kubernetes competitor: there's no scheduler with bin-packing or autoscaling, no CRDs, and no custom orchestration standard to learn. It targets the operator running between 3 and 50 services on 1 to 10 machines who doesn't want to run a control plane the size of Kubernetes to get there.",
  },
  {
    q: 'Is it free?',
    a: 'Yes. Levelrail is Apache 2.0 licensed with one edition: no license key, no enterprise tier, and no feature gated behind a plan. SSO, audit logging, resource-scoped IAM, and scheduled backups all ship in the same binary for everyone.',
  },
  {
    q: 'Can I run it on a single server?',
    a: "Yes, and that's the default. The node agent also runs in single-node mode in the same process as the control plane, talking over an in-memory transport that implements the same interface the networked gRPC transport uses. The code path is identical whether there's one node or ten.",
  },
  {
    q: 'What happens if a deploy fails?',
    a: "Traffic only cuts over to a new container once its readiness probe passes, not once the process merely starts. Rolling, recreate, or blue-green strategies are all gated on that check, and the previous N images stay pinned so garbage collection can never orphan a rollback target, a real bug in some competing platforms.",
  },
  {
    q: 'Does it support multiple servers?',
    a: 'Yes. A second node joins through a one-time enrollment token and gets its own client certificate. Nodes mesh over WireGuard with internal DNS across machines, and managed servers never need an inbound port open, since every agent dials out to the control plane.',
  },
  {
    q: 'Is there an AI or MCP integration?',
    a: "Yes, an MCP server (144 tools, currently beta) wraps the same HTTP API the dashboard runs on: list apps, read logs, get metrics, diagnose a crashloop, trigger a deploy, roll back. It's a read-and-suggest layer on top of the platform API. AI is never part of the reconciliation path itself.",
  },
]
