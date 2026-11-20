# Levelrail Pages deploy

Composite action that installs the Levelrail CLI and deploys a folder to a Pages site. Use a site-scoped token so the pipeline can deploy that one site and nothing else.

```bash
levelrail-cli pages token create docs --label github-actions
```

Save the printed token as a repository secret, then:

```yaml
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
| `version` | no | CLI release tag, default latest stable |

The output `url` is the live URL. The token is passed through the environment and masked in logs.
