import {
  ArrowSquareOutIcon,
  ArrowsClockwiseIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { httpsOrigin, type DomainProgress } from '../../lib/setupWizard'
import { CopyValue, SubStepRow } from './StepChrome'
import { DashboardUrlAction } from './DashboardUrlAction'

/** DomainVerification shows the DNS record to create and the live DNS and HTTPS sub-steps. */
export function DomainVerification({
  domain,
  progress,
  publicIp,
  budgetSpent,
  onRecheck,
}: {
  domain: string
  progress: DomainProgress
  publicIp: string | undefined
  budgetSpent: boolean
  onRecheck: () => void
}) {
  const recordType = publicIp?.includes(':') ? 'AAAA' : 'A'
  const httpsUrl = httpsOrigin(domain)
  const finished = progress.cert === 'ok'

  return (
    <div className="space-y-3 rounded-lg border border-border p-3">
      <div className="space-y-2">
        <p className="text-sm font-medium text-foreground">
          Create this DNS record at your DNS provider
        </p>
        <div className="grid gap-2 text-xs sm:grid-cols-[auto_1fr_1fr]">
          <div className="space-y-1">
            <p className="text-muted-foreground">Type</p>
            <p className="rounded-md bg-muted/50 px-2.5 py-1.5 font-mono">
              {recordType}
            </p>
          </div>
          <div className="min-w-0 space-y-1">
            <p className="text-muted-foreground">Name</p>
            <CopyValue value={domain} label="record name" />
          </div>
          <div className="min-w-0 space-y-1">
            <p className="text-muted-foreground">Value</p>
            {publicIp ? (
              <CopyValue value={publicIp} label="record value" />
            ) : (
              <p className="rounded-md bg-muted/50 px-2.5 py-1.5">
                This server&apos;s public IP (could not be detected, check the
                server step)
              </p>
            )}
          </div>
        </div>
        <p className="text-xs text-muted-foreground">
          If your DNS provider proxies traffic (for example an orange-cloud
          record), turn the proxy off until the certificate is issued.
        </p>
      </div>

      <ol className="divide-y divide-border">
        <SubStepRow
          state="ok"
          title="Domain saved"
          detail={`Routing ${domain} to this dashboard.`}
        />
        <SubStepRow
          state={progress.dns}
          title="DNS points to this server"
          detail={progress.dnsDetail}
        />
        <SubStepRow
          state={progress.cert}
          title="HTTPS certificate issued"
          detail={progress.certDetail}
        />
      </ol>

      {budgetSpent && !finished ? (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-md bg-muted/50 p-2.5">
          <p className="text-xs text-muted-foreground">
            Stopped checking automatically. DNS changes can take a while to
            spread; check again once the record is in place.
          </p>
          <Button type="button" size="sm" variant="outline" onClick={onRecheck}>
            <ArrowsClockwiseIcon />
            Check again
          </Button>
        </div>
      ) : null}

      {finished && httpsUrl ? (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-md bg-green-50 p-2.5 dark:bg-green-950/40">
          <p className="text-xs text-foreground">
            {httpsUrl} is ready. Sign in there and this wizard picks up where
            you left off.
          </p>
          <Button
            size="sm"
            render={<a href={httpsUrl} target="_blank" rel="noreferrer" />}
          >
            Open {httpsUrl}
            <ArrowSquareOutIcon />
          </Button>
          <div className="w-full">
            <DashboardUrlAction httpsUrl={httpsUrl} />
          </div>
        </div>
      ) : null}
    </div>
  )
}
