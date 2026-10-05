---
{
  "layout": "landing",
  "title": "Zero-downtime deploys with Docker: readiness-gated cutover and rollback",
  "description": "How Levelrail ships without downtime: blue-green, rolling or recreate strategies gated on real readiness probes, digest-truthful deploys, a stale-deploy guard, freeze windows and pinned rollback images.",
  "sidebar": false,
  "aside": false,
  "landing": {
    "eyebrow": "Zero-downtime deploys",
    "headline": "A deploy is live only when it is actually ready",
    "sub": "Traffic cuts over after the new container passes a real readiness probe, the previous release stays available, and four guarantees keep the wrong build from serving.",
    "primary": {
      "text": "Read deploy safety",
      "link": "/deploy-safety"
    },
    "secondary": {
      "text": "View on GitHub",
      "link": "https://github.com/glincker/levelrail"
    },
    "install": "curl -fsSL https://levelrail.com/install.sh | sudo sh",
    "shot": {
      "mock": "deploys",
      "alt": "Illustrative rendering of Levelrail deploy history with rollback"
    },
    "cardsHeading": "Four guarantees between a deploy and serving traffic",
    "cards": [
      {
        "title": "Digest-truthful deploys",
        "body": "A deploy is pinned to the image digest it resolved, so a moved tag cannot change what serves. Rollbacks deploy the recorded pinned reference.",
        "link": {
          "text": "Deploy safety",
          "href": "/deploy-safety"
        },
        "visual": [
          {
            "k": "out",
            "t": "web:42 -> sha256:9f2c"
          },
          {
            "k": "ok",
            "t": "pinned reference recorded"
          }
        ]
      },
      {
        "title": "Stale-deploy guard",
        "body": "Automated deploys carry a per-app sequence number and commit order, so an older build still in flight cannot overwrite a newer one. Manual deploys and rollbacks are never rejected.",
        "visual": [
          {
            "k": "out",
            "t": "build #118 finished late"
          },
          {
            "k": "ok",
            "t": "skipped, #119 is newer"
          }
        ]
      },
      {
        "title": "Freeze windows",
        "body": "Block automated deploys on a cron schedule, for example every Friday evening, and release them afterwards.",
        "visual": [
          {
            "k": "cmd",
            "t": "levelrail-cli apps freeze set web --cron \"0 17 * * 5\""
          },
          {
            "k": "ok",
            "t": "weekend freeze active"
          }
        ]
      },
      {
        "title": "Instant rollback window",
        "body": "The previous release is held briefly after cutover, and prior images stay pinned so garbage collection cannot remove a rollback target.",
        "visual": [
          {
            "k": "cmd",
            "t": "levelrail-cli rollback web --image web:41"
          },
          {
            "k": "ok",
            "t": "web is live on web:41"
          }
        ]
      },
      {
        "title": "Three strategies",
        "body": "Blue-green, rolling or recreate, all gated on readiness and liveness probes.",
        "visual": [
          {
            "k": "out",
            "t": "strategy: blue-green"
          },
          {
            "k": "ok",
            "t": "cutover after readiness"
          }
        ]
      },
      {
        "title": "A reason for every state",
        "body": "The reconciler records a status condition with a reason after each pass, so a stuck deploy explains itself.",
        "visual": [
          {
            "k": "out",
            "t": "condition: Progressing"
          },
          {
            "k": "ok",
            "t": "reason: waiting for readiness"
          }
        ]
      }
    ],
    "faq": [
      {
        "q": "What does zero downtime require from my app?",
        "a": "A readiness endpoint that returns success only when the app can serve. Cutover waits for it."
      },
      {
        "q": "What if the new version never becomes ready?",
        "a": "Traffic stays on the current release and the deploy is reported as failed, with logs and a reason."
      },
      {
        "q": "Can a stale build overwrite a newer one?",
        "a": "Not for automated deploys: the stale-deploy guard skips them. Manual deploys and explicit rollbacks always run."
      },
      {
        "q": "Does it work on one server?",
        "a": "Yes. The same cutover logic runs on a single node."
      }
    ],
    "cta": {
      "heading": "Ship without holding your breath",
      "sub": "Read the deploy safety guide or try it on a spare server."
    }
  }
}
---
