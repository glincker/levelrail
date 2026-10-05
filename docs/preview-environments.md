---
{
  "layout": "landing",
  "title": "Preview environments per pull request on your own servers",
  "description": "Levelrail deploys a preview environment for each pull request on GitHub, GitLab, Bitbucket and Gitea, reports status back, and tears it down when the pull request closes.",
  "sidebar": false,
  "aside": false,
  "landing": {
    "eyebrow": "Preview environments",
    "headline": "Every pull request gets its own running copy",
    "sub": "Open or update a pull request and Levelrail deploys a preview. Close it and the preview is torn down, whether or not it merged.",
    "primary": {
      "text": "Set up git integrations",
      "link": "/git-integrations"
    },
    "secondary": {
      "text": "View on GitHub",
      "link": "https://github.com/glincker/levelrail"
    },
    "install": "curl -fsSL https://levelrail.com/install.sh | sudo sh",
    "shot": {
      "mock": "apps",
      "alt": "Illustrative rendering of the Levelrail apps list including preview environments"
    },
    "cardsHeading": "How previews work",
    "cards": [
      {
        "title": "Opened or updated",
        "body": "A new pull request or a new push to it deploys or redeploys a preview copy automatically.",
        "visual": [
          {
            "k": "out",
            "t": "PR #128 opened"
          },
          {
            "k": "ok",
            "t": "preview deployed"
          }
        ]
      },
      {
        "title": "Closed",
        "body": "Closing a pull request tears the preview down regardless of whether it merged.",
        "visual": [
          {
            "k": "out",
            "t": "PR #128 closed"
          },
          {
            "k": "ok",
            "t": "preview torn down"
          }
        ]
      },
      {
        "title": "Status back to your git host",
        "body": "On GitHub, each preview shows as a deployment in the preview environment that moves from in progress to success or failure.",
        "visual": [
          {
            "k": "out",
            "t": "deployment: preview"
          },
          {
            "k": "ok",
            "t": "status: success"
          }
        ]
      },
      {
        "title": "Disposable databases",
        "body": "An app can provision a full, disposable database for every pull request preview that is destroyed with the preview.",
        "visual": [
          {
            "k": "out",
            "t": "ephemeralInPreviews: true"
          },
          {
            "k": "ok",
            "t": "database dropped with preview"
          }
        ]
      },
      {
        "title": "Multi-service previews",
        "body": "A multi-service app fans a preview out the same way a real deploy does, never through a second, drifted code path.",
        "visual": [
          {
            "k": "out",
            "t": "services: web, api, worker"
          },
          {
            "k": "ok",
            "t": "preview ready"
          }
        ]
      },
      {
        "title": "Four git providers",
        "body": "GitHub, GitLab, Bitbucket and Gitea sources all support previews.",
        "link": {
          "text": "Git integrations",
          "href": "/git-integrations"
        },
        "visual": [
          {
            "k": "out",
            "t": "github  gitlab"
          },
          {
            "k": "ok",
            "t": "bitbucket  gitea"
          }
        ]
      }
    ],
    "faq": [
      {
        "q": "Which git hosts support previews?",
        "a": "GitHub, GitLab, Bitbucket and Gitea."
      },
      {
        "q": "Are previews on by default?",
        "a": "Previews are gated on a per-app preview setting, so you turn them on where you want them."
      },
      {
        "q": "What about secrets in previews?",
        "a": "Previews use the app's configuration, so review which environment values a preview should receive before enabling it for untrusted forks."
      },
      {
        "q": "Is this the same as deploy thumbnails?",
        "a": "No. Deploy previews, the thumbnails in deploy history, are a separate feature from per-pull-request preview environments."
      }
    ],
    "cta": {
      "heading": "Review changes on a real URL",
      "sub": "Connect a repository and open a pull request."
    }
  }
}
---
