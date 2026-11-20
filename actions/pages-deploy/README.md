# Levelrail Pages deploy

Composite action that downloads the Levelrail CLI from the GitHub release, verifies it against `checksums.txt`, deploys a folder to a Pages site and, on pull requests, comments the URL. Use a site-scoped token so the pipeline can deploy that one site and nothing else.

```bash
levelrail-cli pages token create --site docs --ttl 90d --label github-actions
```

The token is printed once. Save it as a repository secret, then:

```yaml
permissions:
  contents: read
  pull-requests: write
steps:
  - uses: actions/checkout@v4
  - id: pages
    uses: GLINCKER/levelrail/actions/pages-deploy@v1.5.7
    with:
      version: v1.5.7
      site: docs
      dir: ./dist
      server: https://levelrail.example.com
      token: ${{ secrets.LEVELRAIL_PAGES_TOKEN }}
  - run: echo "Live at ${{ steps.pages.outputs.url }}"
```

| Input | Required | Meaning |
| --- | --- | --- |
| `site` | yes | Pages site name |
| `dir` | no | Folder or zip to publish, default `.` |
| `server` | yes | Base URL of your server |
| `token` | yes | Site deploy token |
| `version` | no | CLI release tag, default latest |
| `comment` | no | Comment the URL on pull requests, default `true` |
| `github-token` | no | Used for the release download and the comment, default `github.token` |

The output `url` is the live URL. The token is passed through the environment and masked in logs. See `docs/pages-ci.md` for other CI systems.
