---
{
  "layout": "landing",
  "title": "Contribute to Levelrail: open source deployment platform",
  "description": "Help build Levelrail: try it and report what breaks, pick a good first issue, improve docs, add templates or test multi-node. Apache 2.0, active development.",
  "sidebar": false,
  "aside": false,
  "landing": {
    "eyebrow": "Contribute",
    "headline": "Help build a deployment platform you would want to use",
    "sub": "Levelrail is open source and moving fast. The most valuable contributions right now are real usage, bug reports and focused pull requests.",
    "primary": {
      "text": "Browse good first issues",
      "link": "https://github.com/glincker/levelrail/labels/good%20first%20issue"
    },
    "secondary": {
      "text": "View on GitHub",
      "link": "https://github.com/glincker/levelrail"
    },
    "stats": [
      {
        "value": "Apache 2.0",
        "label": "license"
      },
      {
        "value": "Go + React",
        "label": "stack"
      },
      {
        "value": "1 edition",
        "label": "no paid tier"
      }
    ],
    "cardsHeading": "Ways to help",
    "cards": [
      {
        "title": "Try it and tell us what breaks",
        "body": "Install on a spare server, deploy something real and open an issue with what you saw. Reports from real workloads are the most useful thing right now.",
        "link": {
          "text": "Open an issue",
          "href": "https://github.com/glincker/levelrail/issues"
        },
        "visual": [
          {
            "k": "cmd",
            "t": "curl -fsSL https://levelrail.com/install.sh | sudo sh"
          },
          {
            "k": "ok",
            "t": "found something? open an issue"
          }
        ]
      },
      {
        "title": "Pick a good first issue",
        "body": "Issues labeled good first issue and help wanted are scoped for newcomers. Read the contributing guide first for branch and commit conventions.",
        "link": {
          "text": "Contributing guide",
          "href": "https://github.com/glincker/levelrail/blob/main/CONTRIBUTING.md"
        },
        "visual": [
          {
            "k": "out",
            "t": "label: good first issue"
          },
          {
            "k": "ok",
            "t": "label: help wanted"
          }
        ]
      },
      {
        "title": "Improve the docs",
        "body": "Every docs page has an edit link. Fixes to unclear steps, missing flags and broken examples land fast.",
        "visual": [
          {
            "k": "out",
            "t": "Edit this page on GitHub"
          },
          {
            "k": "ok",
            "t": "docs PRs welcome"
          }
        ]
      },
      {
        "title": "Test multi-node",
        "body": "Enrollment, cordon and drain are verified against real Docker daemons. Cross-host transport and the WireGuard mesh need more real-world testing.",
        "link": {
          "text": "Multi-node quickstart",
          "href": "/multi-node-quickstart"
        },
        "visual": [
          {
            "k": "out",
            "t": "enroll, cordon, drain  verified"
          },
          {
            "k": "ok",
            "t": "mesh needs testers"
          }
        ]
      },
      {
        "title": "Talk to us",
        "body": "Ask a quick question in the Levelrail forum on the GLINR Discord, or start a GitHub discussion.",
        "link": {
          "text": "Join the Discord",
          "href": "https://discord.gg/Ar5pcaZB99"
        },
        "visual": [
          {
            "k": "out",
            "t": "#levelrail forum"
          },
          {
            "k": "ok",
            "t": "GitHub Discussions"
          }
        ]
      },
      {
        "title": "Star the repo",
        "body": "A star helps other operators find the project. It costs nothing and it does help.",
        "link": {
          "text": "Star on GitHub",
          "href": "https://github.com/glincker/levelrail"
        },
        "visual": [
          {
            "k": "out",
            "t": "github.com/glincker/levelrail"
          },
          {
            "k": "ok",
            "t": "star it"
          }
        ]
      }
    ],
    "stepsHeading": "Your first pull request",
    "steps": [
      {
        "title": "Open an issue first for anything bigger than a small fix",
        "body": "The reconciler, agent transport and database schema have deliberate design constraints worth discussing before you write code."
      },
      {
        "title": "Branch and commit with the conventions",
        "body": "Branch names like fix/short-description and conventional commits. Hooks check formatting, lint, tests and dashes."
      },
      {
        "title": "Keep each pull request to one logical change",
        "body": "Smaller, focused pull requests get reviewed faster."
      }
    ],
    "faq": [
      {
        "q": "Do I need to sign a CLA?",
        "a": "The contributing guide is the source of truth for contribution terms. Read it before opening your first pull request."
      },
      {
        "q": "What languages is it written in?",
        "a": "Go for the control plane and agent, and TypeScript with React for the dashboard."
      },
      {
        "q": "Where do I report a security issue?",
        "a": "Follow the security policy in the repository rather than opening a public issue."
      },
      {
        "q": "Is there a roadmap?",
        "a": "Yes, the roadmap page lists phases and current status."
      }
    ],
    "cta": {
      "heading": "Come build it with us",
      "sub": "Every issue, fix and star moves it forward."
    }
  }
}
---
