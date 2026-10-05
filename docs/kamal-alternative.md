---
{
  "layout": "landing",
  "title": "Kamal alternative with a dashboard, history and built-in observability",
  "description": "Levelrail is an open source Kamal alternative: the same zero-downtime idea with a control plane, dashboard, deploy history, rollback and built-in metrics and logs.",
  "sidebar": false,
  "aside": false,
  "landing": {
    "eyebrow": "Kamal alternative",
    "headline": "Zero-downtime deploys, plus a control plane you can see",
    "sub": "Kamal is deliberately minimal: no server component, just SSH from your machine or CI. Levelrail keeps readiness-gated cutover and adds a dashboard, history, rollback and observability.",
    "primary": {
      "text": "Get started",
      "link": "/getting-started"
    },
    "secondary": {
      "text": "View on GitHub",
      "link": "https://github.com/glincker/levelrail"
    },
    "install": "curl -fsSL https://levelrail.com/install.sh | sudo sh",
    "shot": {
      "mock": "overview",
      "alt": "Illustrative rendering of the Levelrail app overview with live charts and deploy markers"
    },
    "cardsHeading": "What a control plane adds",
    "cards": [
      {
        "title": "State you can inspect",
        "body": "Desired state lives in a database and a reconciler records a status condition with a reason after every pass, so you can see why something is not ready.",
        "visual": [
          {
            "k": "out",
            "t": "app web  desired: web:42"
          },
          {
            "k": "out",
            "t": "observed: web:42"
          },
          {
            "k": "ok",
            "t": "Ready, probe passed"
          }
        ]
      },
      {
        "title": "History and rollback",
        "body": "Every deploy is recorded with its pinned image. Rolling back is switching to an image that is still there.",
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
        "title": "No SSH from CI",
        "body": "Servers run an agent that dials out to the control plane, so CI talks to the API with a scoped token instead of holding SSH keys.",
        "visual": [
          {
            "k": "out",
            "t": "token ability: deploy"
          },
          {
            "k": "ok",
            "t": "no SSH key in CI"
          }
        ]
      }
    ],
    "compare": {
      "heading": "Kamal and Levelrail, side by side",
      "intro": "Kamal is a sharp tool for people who want the fewest moving parts. These are the design differences.",
      "left": "Kamal",
      "right": "Levelrail",
      "rows": [
        {
          "label": "How it manages servers",
          "left": "SSH from your machine or CI to each server",
          "right": "Agent per node that dials out over mTLS"
        },
        {
          "label": "Orchestration",
          "left": "None: it runs Docker commands per deploy",
          "right": "Level-triggered reconciler"
        },
        {
          "label": "Ingress",
          "left": "kamal-proxy",
          "right": "Caddy embedded in the control plane"
        },
        {
          "label": "Runs as",
          "left": "A command line tool, no server component",
          "right": "A control plane, an agent per extra node, a dashboard and a CLI"
        },
        {
          "label": "Observability",
          "left": "Bring your own",
          "right": "Built in, node-local"
        }
      ],
      "more": {
        "text": "Read the full comparison",
        "href": "/comparison"
      }
    },
    "steps": [
      {
        "title": "Install the control plane",
        "body": "One script on a Linux server."
      },
      {
        "title": "Deploy an image",
        "body": "Use the CLI or the dashboard and watch the readiness gate."
      },
      {
        "title": "Wire CI to the API",
        "body": "Use a scoped API token from GitHub Actions or any pipeline."
      }
    ],
    "stepsHeading": "From a deploy script to a platform",
    "faq": [
      {
        "q": "Is Levelrail heavier than Kamal?",
        "a": "Yes, by design: it is a control plane with a database and a dashboard, while Kamal has no server component. The control plane idles at a small footprint, which the case studies page measures."
      },
      {
        "q": "Can I deploy from CI without SSH?",
        "a": "Yes. CI calls the API or CLI with a scoped token. See the GitHub Actions guide."
      },
      {
        "q": "Is there an importer for Kamal?",
        "a": "No. You point Levelrail at the same images and recreate configuration."
      },
      {
        "q": "Is it free?",
        "a": "Yes. Apache 2.0, one edition."
      }
    ],
    "cta": {
      "heading": "Keep the zero-downtime idea, add the dashboard",
      "sub": "Try it on one server beside your current setup."
    }
  }
}
---
