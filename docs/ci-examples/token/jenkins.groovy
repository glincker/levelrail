// Jenkinsfile
// Pin the CLI with LEVELRAIL_VERSION=vX.Y.Z on the installer step for reproducible builds.
// Build, push and deploy with a stored Levelrail token. Create it once with
//   levelrail-cli tokens create --name ci-my-app --preset deployer
// then add two Secret text credentials under Manage Jenkins > Credentials with
// the ids levelrail-api-url and levelrail-token.
pipeline {
  agent any
  environment {
    IMAGE = "registry.example.com/team/my-app:${env.GIT_COMMIT}"
    LEVELRAIL_API_URL = credentials('levelrail-api-url')
    LEVELRAIL_TOKEN = credentials('levelrail-token')
    LEVELRAIL_CLI_INSTALL_DIR = "${env.WORKSPACE}/.bin"
    CI = 'true'
  }
  stages {
    stage('Build and push') {
      steps {
        sh 'docker build -t "$IMAGE" .'
        sh 'docker push "$IMAGE"'
      }
    }
    stage('Deploy') {
      steps {
        sh 'curl -fsSL https://levelrail.com/install-cli.sh | sh'
        sh '''
          export PATH="$LEVELRAIL_CLI_INSTALL_DIR:$PATH"
          levelrail-cli apps deploy my-app --image "$IMAGE" --idempotency-key "jenkins-$BUILD_TAG"
          levelrail-cli apps wait my-app --timeout 5m
        '''
      }
    }
  }
}
