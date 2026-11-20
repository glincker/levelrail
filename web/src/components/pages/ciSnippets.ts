/** Snippets reference the secret by variable name so the token never lands in a copied file. */
export function ciSnippets(site: string, server: string) {
  return {
    github: `- uses: GLINCKER/levelrail/actions/pages-deploy@main
  with:
    site: ${site}
    dir: ./dist
    server: ${server}
    token: \${{ secrets.LEVELRAIL_PAGES_TOKEN }}`,
    gitlab: `deploy:
  script:
    - curl -fsSL https://github.com/glincker/levelrail/releases/latest/download/install-cli.sh | sh
    - levelrail-cli pages deploy ./dist --name ${site}
  variables:
    APP_API_URL: ${server}
    APP_API_TOKEN: $LEVELRAIL_PAGES_TOKEN`,
    curl: `curl -fsS -X POST "${server}/api/v1/apps/${site}/uploads" \\
  -H "Authorization: Bearer $LEVELRAIL_PAGES_TOKEN" \\
  -H "Content-Type: application/zip" --data-binary @site.zip`,
  }
}
