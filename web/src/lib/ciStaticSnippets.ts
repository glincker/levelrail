// Pipeline files for a static site: no container, the folder is published with
// the CLI. The token always comes from a secret variable, never the file.
const INSTALL = 'curl -fsSL https://levelrail.com/install-cli.sh | sh'

export interface StaticSnippet {
  target: string
  content: string
}

function deployLine(site: string) {
  return `levelrail-cli pages deploy ./dist --name ${site}`
}

const BUILDERS: Record<string, (site: string) => StaticSnippet> = {
  github: (site) => ({
    target: '.github/workflows/deploy.yml',
    content: `# .github/workflows/deploy.yml
# Add LEVELRAIL_TOKEN as a repository secret and LEVELRAIL_API_URL as a repository variable.
name: Deploy site
on:
  push:
    branches: [main]
jobs:
  deploy:
    runs-on: ubuntu-latest
    env:
      LEVELRAIL_API_URL: \${{ vars.LEVELRAIL_API_URL }}
      LEVELRAIL_TOKEN: \${{ secrets.LEVELRAIL_TOKEN }}
      LEVELRAIL_CLI_INSTALL_DIR: /home/runner/.local/bin
    steps:
      - uses: actions/checkout@v4
      # Build the site here if it has a build step, so ./dist exists.
      - name: Publish to Levelrail
        run: |
          ${INSTALL}
          export PATH="$LEVELRAIL_CLI_INSTALL_DIR:$PATH"
          ${deployLine(site)}
`,
  }),
  gitlab: (site) => ({
    target: '.gitlab-ci.yml',
    content: `# .gitlab-ci.yml
# Add LEVELRAIL_TOKEN (masked) and LEVELRAIL_API_URL under Settings > CI/CD > Variables.
deploy:
  stage: deploy
  image: alpine:3
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
  variables:
    LEVELRAIL_CLI_INSTALL_DIR: /usr/local/bin
  script:
    # Build the site first if it has a build step, so ./dist exists.
    - apk add --no-cache curl
    - ${INSTALL}
    - ${deployLine(site)}
`,
  }),
  circleci: (site) => ({
    target: '.circleci/config.yml',
    content: `# .circleci/config.yml
# Put LEVELRAIL_TOKEN and LEVELRAIL_API_URL in the levelrail context.
version: 2.1

jobs:
  deploy:
    docker:
      - image: cimg/base:stable
    environment:
      LEVELRAIL_CLI_INSTALL_DIR: /home/circleci/bin
    steps:
      - checkout
      # Build the site here if it has a build step, so ./dist exists.
      - run:
          name: Publish to Levelrail
          command: |
            ${INSTALL}
            export PATH="$LEVELRAIL_CLI_INSTALL_DIR:$PATH"
            ${deployLine(site)}

workflows:
  main:
    jobs:
      - deploy:
          context: levelrail
          filters:
            branches:
              only: main
`,
  }),
  jenkins: (site) => ({
    target: 'Jenkinsfile',
    content: `// Jenkinsfile
// Add two Secret text credentials with the ids levelrail-api-url and levelrail-token.
pipeline {
  agent any
  environment {
    LEVELRAIL_API_URL = credentials('levelrail-api-url')
    LEVELRAIL_TOKEN = credentials('levelrail-token')
    LEVELRAIL_CLI_INSTALL_DIR = "\${env.WORKSPACE}/.bin"
  }
  stages {
    stage('Publish') {
      steps {
        // Build the site first if it has a build step, so ./dist exists.
        sh '${INSTALL}'
        sh '''
          export PATH="$LEVELRAIL_CLI_INSTALL_DIR:$PATH"
          ${deployLine(site)}
        '''
      }
    }
  }
}
`,
  }),
  buildkite: (site) => ({
    target: '.buildkite/pipeline.yml',
    content: `# .buildkite/pipeline.yml
# Keep LEVELRAIL_API_URL and LEVELRAIL_TOKEN in the agent environment hook or Pipeline Settings, never in this file.
steps:
  - label: "Publish to Levelrail"
    branches: "main"
    env:
      LEVELRAIL_CLI_INSTALL_DIR: "$BUILDKITE_BUILD_CHECKOUT_PATH/.bin"
    command: |
      # Build the site first if it has a build step, so ./dist exists.
      ${INSTALL}
      export PATH="$$LEVELRAIL_CLI_INSTALL_DIR:$$PATH"
      ${deployLine(site)}
`,
  }),
}

/** Returns the static-site pipeline for a provider id, or undefined for one we have no file for. */
export function staticSiteSnippet(
  providerId: string,
  site: string,
): StaticSnippet | undefined {
  return BUILDERS[providerId]?.(site)
}
