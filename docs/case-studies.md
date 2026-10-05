---
{
  "layout": "landing",
  "title": "Case studies: measured results, not testimonials",
  "description": "Real Levelrail measurements: idle CPU and memory on a production-test VPS, a 500-app benchmark, and multi-node verification against real Docker daemons, plus what is not proven yet.",
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
        "value": "0.7%",
        "label": "CPU at idle, production-test VPS"
      },
      {
        "value": "146 MB",
        "label": "RAM at idle, same server"
      },
      {
        "value": "0.06",
        "label": "load average, same server"
      },
      {
        "value": "500",
        "label": "apps in the idle benchmark"
      }
    ],
    "cardsHeading": "What has been verified",
    "cards": [
      {
        "title": "Light at idle on a real VPS",
        "body": "On the production-test VPS, the control plane sat at 0.7% CPU, 146 MB of memory and a load average of 0.06, read from ps and uptime.",
        "visual": [
          {
            "k": "out",
            "t": "cpu     0.7%"
          },
          {
            "k": "out",
            "t": "memory  146 MB"
          },
          {
            "k": "ok",
            "t": "load    0.06"
          }
        ],
        "icon": "gauge"
      },
      {
        "title": "Footprint as apps grow",
        "body": "In the idle benchmark, going from 0 to 500 suspended apps moved resident memory from about 68 MB to about 91 MB and CPU from 0.03% to 1.6% on a development build. The method is reproducible.",
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
          "Levelrail has no stable release, and single node is the best tested path. The WireGuard mesh and cross-host remote transport beyond the join flow are still unverified, and public ACME issuance has had less field verification than the rest of the ingress. The feature status page lists the evidence behind each area, area by area.",
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
        "a": "The VPS figures were read from ps and uptime on the production-test server. The 500-app figures come from a documented benchmark script you can run yourself."
      },
      {
        "q": "Are these numbers comparable to other platforms?",
        "a": "They were not measured against other platforms here, so this page makes no comparative claim. Measure your own setup with the same method if you want a comparison."
      }
    ],
    "cta": {
      "heading": "Add your results",
      "sub": "Run it, measure it, and tell us what you find."
    }
  }
}
---
