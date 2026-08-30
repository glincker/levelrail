import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import {
  ArrowSquareOutIcon,
  CheckCircleIcon,
  ArrowCircleUpIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from '../../components/ui/card'
import { updatesQueryOptions } from '../../queries/updates'
import type { UpdateStatus } from '../../queries/updates'
import { PageSpinner } from '../../components/ui/page-spinner'

export const Route = createFileRoute('/settings/updates')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(updatesQueryOptions()),
  component: UpdatesSettingsPage,
  pendingComponent: PageSpinner,
})

function UpdatesSettingsPage() {
  const { data: status } = useSuspenseQuery(updatesQueryOptions())

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-foreground">Updates</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Current version and available releases.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Version</CardTitle>
          <CardDescription>
            Compared against github.com/glincker/levelrail's published
            releases.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between py-1.5 text-sm">
            <span className="text-foreground">Running version</span>
            <span className="font-mono text-muted-foreground">
              {status.current_version}
            </span>
          </div>
          <UpdateStatusRow status={status} />
        </CardContent>
      </Card>
    </div>
  )
}

function UpdateStatusRow({ status }: { status: UpdateStatus }) {
  if (status.latest_version === null) {
    return (
      <p className="text-sm text-muted-foreground">
        No releases published yet.
      </p>
    )
  }

  if (!status.update_available) {
    return (
      <div className="inline-flex items-center gap-1.5 text-sm text-green-700 dark:text-green-400">
        <CheckCircleIcon className="size-4" />
        You&apos;re on the latest version.
      </div>
    )
  }

  return (
    <div className="space-y-2">
      <div className="inline-flex items-center gap-1.5 text-sm text-foreground">
        <ArrowCircleUpIcon className="size-4" />
        A new version is available: {status.latest_version}
      </div>
      {status.release_url && (
        <a
          href={status.release_url}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 text-sm text-primary hover:underline"
        >
          View release
          <ArrowSquareOutIcon className="size-3.5" />
        </a>
      )}
    </div>
  )
}
