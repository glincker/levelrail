import { useQuery } from '@tanstack/react-query'
import { LockKeyIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  dashboardUrlQueryOptions,
  useUpdateDashboardUrl,
} from '../../queries/dashboardUrl'

/** DashboardUrlAction saves the verified https address as the dashboard URL, which turns off plain-HTTP sign-in. */
export function DashboardUrlAction({ httpsUrl }: { httpsUrl: string }) {
  const { data } = useQuery(dashboardUrlQueryOptions())
  const update = useUpdateDashboardUrl()

  if (data?.dashboard_url === httpsUrl) {
    return (
      <p className="flex items-center gap-1.5 text-xs text-foreground">
        <LockKeyIcon className="size-4 text-green-600 dark:text-green-400" />
        {httpsUrl} is the dashboard URL. Sign-in over plain HTTP is off.
      </p>
    )
  }

  const onHttpsOrigin = window.location.origin === httpsUrl
  return (
    <div className="space-y-1.5">
      <p className="text-xs text-muted-foreground">
        {onHttpsOrigin
          ? 'Save it as the dashboard URL to turn off sign-in over plain HTTP.'
          : `Last step: open ${httpsUrl}, sign in, and save it as the dashboard URL from there. Saving it from a plain-HTTP page is refused so you cannot lock yourself out.`}
      </p>
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={update.isPending}
        onClick={() =>
          update.mutate(
            { dashboard_url: httpsUrl },
            {
              onSuccess: () => {
                // The session cookie does not carry over, so sign in again on the https origin.
                if (!onHttpsOrigin) window.location.assign(`${httpsUrl}/`)
              },
            },
          )
        }
      >
        <LockKeyIcon />
        Use as dashboard URL
      </Button>
      {update.error ? (
        <p className="text-xs text-destructive">{update.error.message}</p>
      ) : null}
    </div>
  )
}
