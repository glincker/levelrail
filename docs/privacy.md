---
{
  "layout": "landing",
  "title": "Privacy and data handling: what Levelrail stores and sends",
  "description": "Where your data lives with Levelrail: secrets, metrics and logs stay on your own servers, the product has no usage telemetry, and the only built-in outbound check is an opt-in update lookup on GitHub.",
  "sidebar": false,
  "aside": false,
  "landing": {
    "eyebrow": "Privacy and data handling",
    "headline": "Your data stays on your servers",
    "sub": "Levelrail is self-hosted. Secrets, metrics and logs live on infrastructure you control, and the platform does not report usage to us.",
    "primary": {
      "text": "Read the security overview",
      "link": "/security"
    },
    "secondary": {
      "text": "Review the source",
      "link": "https://github.com/glincker/levelrail"
    },
    "cardsHeading": "The short version",
    "cards": [
      {
        "title": "Secrets are encrypted per app",
        "body": "Each app has its own data encryption key wrapped by a master key held only by the control plane. Agents receive decrypted environment values when a container is created and do not persist them.",
        "link": {
          "text": "Master key rotation",
          "href": "/master-key-rotation"
        },
        "icon": "lockkey"
      },
      {
        "title": "Metrics and logs stay node-local",
        "body": "Each node keeps its own time series and log store. The control plane queries nodes on demand instead of shipping telemetry to a central service or a third party.",
        "link": {
          "text": "Observability",
          "href": "/observability"
        },
        "icon": "harddrives"
      },
      {
        "title": "No usage telemetry",
        "body": "A search of the Go source for analytics, usage reporting and crash reporting libraries finds none. The code is open, so you can check it yourself.",
        "link": {
          "text": "Source code",
          "href": "https://github.com/glincker/levelrail"
        },
        "icon": "eye"
      }
    ],
    "prose": [
      {
        "heading": "Outbound connections",
        "paragraphs": [
          "Levelrail's own code contains no hardcoded calls to servers operated by us. The one built-in lookup is an update check against GitHub's releases API, and it only runs when an operator opts in to automatic update checks. It checks and records the result and never applies an update by itself.",
          "Everything else that leaves your network is something you configure: git providers for webhooks and clones, an ACME certificate authority for TLS, container registries, cloud provider APIs for node provisioning, object storage for backups, and the alert channels you set up."
        ]
      },
      {
        "heading": "Agents and nodes",
        "paragraphs": [
          "Each node agent dials out to the control plane over mutual TLS. Managed servers need no inbound port, and the agent talks to the local Docker Engine API rather than shelling out."
        ]
      },
      {
        "heading": "AI features",
        "paragraphs": [
          "AI is a read and suggest layer on top of the API and is never in the reconciliation path. Where AI features are used, they call the model provider you configure."
        ]
      },
      {
        "heading": "This website",
        "paragraphs": [
          "The documentation site's own code loads no analytics scripts and sets no cookies, and its fonts are served from the same origin. Like any web host, whoever serves the files can see ordinary server logs.",
          "This page describes how the software behaves, based on the source at the time of writing, and is not a legal contract. For a vulnerability report, follow the project's security policy on GitHub."
        ]
      }
    ],
    "faq": [
      {
        "q": "Does Levelrail send telemetry to GLINCKER?",
        "a": "No. The software has no usage reporting. Its metrics collector is node-local and exists to power your own dashboards and alerts."
      },
      {
        "q": "Does it phone home?",
        "a": "Only if you opt in to update checks, which query GitHub's releases API. Nothing else in the source calls a host operated by the project."
      },
      {
        "q": "Where are my secrets stored?",
        "a": "In the control plane database, encrypted with per-app keys wrapped by a master key. Agents receive them at container creation time and do not persist them."
      },
      {
        "q": "How do I report a security issue?",
        "a": "Follow the security policy in the GitHub repository rather than opening a public issue."
      }
    ],
    "cta": {
      "heading": "Verify it yourself",
      "sub": "The whole platform is Apache 2.0 and public. Read the code, run it, and trace what it connects to."
    }
  }
}
---
