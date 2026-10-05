---
{
  "layout": "landing",
  "title": "Case studies: measured results, not testimonials",
  "description": "Real Levelrail measurements: a 500-app idle benchmark and multi-node verification against real Docker daemons, plus what is not proven yet.",
  "sidebar": false,
  "aside": false,
  "landing": {
    "eyebrow": "Case studies",
    "headline": "Measured results, stated honestly",
    "sub": "Levelrail is pre-release and has no customer logos yet, so instead of testimonials this page shows what has actually been measured and verified, and what has not.",
    "primary": {
      "text": "Run your own test",
      "link": "/getting-started"
    },
    "secondary": {
      "text": "View on GitHub",
      "link": "https://github.com/glincker/levelrail"
    },
    "stats": [
      {
        "value": "68 MB",
        "label": "RSS at 0 apps, dev build"
      },
      {
        "value": "91 MB",
        "label": "RSS at 500 suspended apps"
      },
      {
        "value": "1.6%",
        "label": "CPU at 500 suspended apps"
      },
      {
        "value": "500",
        "label": "apps in the idle benchmark"
      }
    ],
    "cardsHeading": "What has been verified",
    "cards": [
      {
        "title": "Footprint as apps grow",
        "body": "In the idle benchmark, going from 0 to 500 suspended apps moved resident memory from about 68 MB to about 91 MB and CPU from 0.03% to 1.6%. It ran on an Apple M4 Max with a development build and no containers running, so expect different numbers on a Linux VPS. The method is reproducible.",
        "link": {
          "text": "Performance method and numbers",
          "href": "/performance"
        },
        "visual": [
          {
            "k": "out",
            "t": "0 apps    68 MB   0.03%"
          },
          {
            "k": "out",
            "t": "100 apps  83 MB   0.30%"
          },
          {
            "k": "ok",
            "t": "500 apps  91 MB   1.60%"
          }
        ],
        "icon": "chart"
      },
      {
        "title": "Multi-node against real daemons",
        "body": "Enrollment, cordon, drain with real container relocation and explicit placement pinning were run with two real agent processes, each with its own Docker daemon.",
        "link": {
          "text": "Multi-node quickstart",
          "href": "/multi-node-quickstart"
        },
        "visual": [
          {
            "k": "out",
            "t": "enroll    ok"
          },
          {
            "k": "out",
            "t": "cordon    ok"
          },
          {
            "k": "ok",
            "t": "drain, containers relocated"
          }
        ],
        "icon": "network"
      }
    ],
    "prose": [
      {
        "heading": "What is not proven yet",
        "paragraphs": [
          "Levelrail has no stable release, and single node is the best tested path. The WireGuard mesh and cross-host remote transport beyond the join flow are still unverified, there is no Linux release-build idle measurement yet, and public ACME has one live run with renewal and wildcards unverified. The feature status page lists the evidence behind each area.",
          "Fewer real-world deployments also means less community knowledge than projects that have existed for years."
        ]
      },
      {
        "heading": "Be the first case study",
        "paragraphs": [
          "If you run Levelrail on a real workload, we would like to hear what broke and what worked. Open a discussion or an issue on GitHub, and if you are happy to be named, we will write it up with your numbers."
        ]
      }
    ],
    "faq": [
      {
        "q": "Why are there no customer testimonials?",
        "a": "Because the project is pre-release and has no public production customers yet. This page only shows measurements and verifications that exist."
      },
      {
        "q": "Where do the idle numbers come from?",
        "a": "From a documented benchmark script (`scripts/bench-idle.sh`) you can run yourself. The [performance page](/performance) lists the method, the machine and the caveats."
      },
      {
        "q": "Are these numbers comparable to other platforms?",
        "a": "They were not measured against other platforms here, so this page makes no comparative claim. Measure your own setup with the same method if you want a comparison."
      }
    ],
    "cta": {
      "heading": "Add your results",
      "sub": "Run it, measure it, and tell us what you find."
    },
    "related": [
      {
        "text": "Coolify alternative",
        "link": "/coolify-alternative"
      },
      {
        "text": "Dokploy alternative",
        "link": "/dokploy-alternative"
      },
      {
        "text": "Vercel alternative",
        "link": "/vercel-alternative"
      },
      {
        "text": "Heroku alternative",
        "link": "/heroku-alternative"
      },
      {
        "text": "Railway alternative",
        "link": "/railway-alternative"
      },
      {
        "text": "Pricing",
        "link": "/pricing"
      },
      {
        "text": "Privacy",
        "link": "/privacy"
      },
      {
        "text": "Demo",
        "link": "/demo"
      },
      {
        "text": "Full comparison",
        "link": "/comparison"
      }
    ]
  }
}
---
