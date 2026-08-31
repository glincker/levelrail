import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'

// CreateAppFromGitFields' own progress/error alerts for its three-step
// submit sequence (create app, connect git source, trigger build). Split
// out once the parent form crossed a comfortable single-file size, the
// same reasoning GitBuildSourceFields' own doc comment gives for its
// split. Takes plain booleans/strings rather than the mutation objects
// themselves, so this stays a pure presentational component independent
// of which mutation library backs the parent's submit flow.
export function GitDeployStatusAlerts({
  connectingSource,
  triggeringBuild,
  createAppError,
  connectSourceError,
  buildError,
}: {
  connectingSource: boolean
  triggeringBuild: boolean
  createAppError?: string
  connectSourceError?: string
  buildError?: string
}) {
  return (
    <>
      {connectingSource ? (
        <Alert>
          <AlertDescription>Connecting the git source...</AlertDescription>
        </Alert>
      ) : null}

      {triggeringBuild ? (
        <Alert>
          <AlertDescription>Triggering the build...</AlertDescription>
        </Alert>
      ) : null}

      {createAppError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{createAppError}</AlertDescription>
        </Alert>
      ) : null}

      {connectSourceError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>
            App created, but connecting the git source failed:{' '}
            {connectSourceError}. The build below still runs; connect a git
            source afterward from the app&apos;s Overview page for automatic
            deploys on push.
          </AlertDescription>
        </Alert>
      ) : null}

      {buildError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>
            App created, but triggering the build failed: {buildError}. Fix
            the fields above and submit again to retry.
          </AlertDescription>
        </Alert>
      ) : null}
    </>
  )
}
