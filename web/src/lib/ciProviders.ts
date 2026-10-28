// Provider catalogue for the CI deploy trust form and its copyable setup
// snippets. The job lines mirror docs/ci-examples so docs and dashboard agree.

export type CIProvider = 'github' | 'gitlab' | 'circleci'

export const CI_PROVIDERS: readonly CIProvider[] = [
  'github',
  'gitlab',
  'circleci',
]

export function ciProviderOf(value: string | undefined): CIProvider {
  return value === 'gitlab' || value === 'circleci' ? value : 'github'
}

export interface CIJobSnippet {
  language: 'bash' | 'yaml'
  code: string
}

// jobSnippet is the part of the pipeline that trades the job identity for a
// deploy token. apiUrl is the control plane address the token is issued for.
export function jobSnippet(
  provider: CIProvider,
  app: string,
  apiUrl: string,
): CIJobSnippet {
  switch (provider) {
    case 'gitlab':
      return {
        language: 'yaml',
        code: [
          'deploy:',
          '  id_tokens:',
          '    LEVELRAIL_ID_TOKEN:',
          `      aud: ${apiUrl}`,
          '  script:',
          `    - LEVELRAIL_TOKEN=$(levelrail-cli apps ci token ${app} --provider gitlab)`,
          '    - export LEVELRAIL_TOKEN',
          `    - levelrail-cli apps deploy ${app} --image "$PINNED"`,
          `    - levelrail-cli apps wait ${app} --timeout 5m`,
        ].join('\n'),
      }
    case 'circleci':
      return {
        language: 'bash',
        code: [
          `LEVELRAIL_ID_TOKEN=$(circleci run oidc get --claims '{"aud":"${apiUrl}"}')`,
          'export LEVELRAIL_ID_TOKEN',
          `LEVELRAIL_TOKEN=$(levelrail-cli apps ci token ${app} --provider circleci)`,
          'export LEVELRAIL_TOKEN',
          `levelrail-cli apps deploy ${app} --image "$PINNED"`,
          `levelrail-cli apps wait ${app} --timeout 5m`,
        ].join('\n'),
      }
    case 'github':
      return {
        language: 'bash',
        code: [
          '# job needs: permissions: id-token: write',
          `LEVELRAIL_TOKEN=$(levelrail-cli apps ci token ${app} --provider github)`,
          'export LEVELRAIL_TOKEN',
          `levelrail-cli apps deploy ${app} --image "$PINNED"`,
          `levelrail-cli apps wait ${app} --timeout 5m`,
        ].join('\n'),
      }
  }
}
