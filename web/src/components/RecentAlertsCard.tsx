import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import {
  BellRingingIcon,
  CaretDownIcon,
  CaretRightIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useAlertHistory } from '../queries/alertNoise'
import type { AlertHistoryEntry } from '../types/alertNoise'
import { RecentChanges } from './RecentChanges'
import { SilenceRuleMenu } from './SilenceRuleMenu'

const ROW_CAP = 5

function AlertRow({ entry }: { entry: AlertHistoryEntry }) {
  const [open, setOpen] = useState(false)
  return (
    <div className="py-2 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 items-start gap-1">
          {entry.app ? (
            <button
              type="button"
              aria-expanded={open}
              aria-label={`${open ? 'Hide' : 'Show'} what changed before ${entry.rule_name} fired`}
              onClick={() => {
                setOpen((v) => !v)
              }}
              className="mt-0.5 inline-flex size-5 shrink-0 items-center justify-center rounded text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60"
            >
              {open ? (
                <CaretDownIcon className="size-3.5" aria-hidden="true" />
              ) : (
                <CaretRightIcon className="size-3.5" aria-hidden="true" />
              )}
            </button>
          ) : null}
          <div className="min-w-0">
            <p className="truncate text-sm font-medium text-foreground">
              {entry.rule_name}
            </p>
            <p className="text-xs text-muted-foreground">
              {entry.app ?? 'platform'} &middot;{' '}
              {new Date(entry.at).toLocaleString()}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Badge
            variant={
              entry.outcome === 'sent'
                ? 'success'
                : entry.outcome === 'failed'
                  ? 'destructive'
                  : 'muted'
            }
          >
            {entry.outcome}
          </Badge>
          {entry.app ? (
            <SilenceRuleMenu
              appName={entry.app}
              ruleId={entry.rule_id}
              ruleName={entry.rule_name}
            />
          ) : null}
        </div>
      </div>
      {open && entry.app ? (
        <RecentChanges
          app={entry.app}
          until={entry.at}
          className="mt-2 rounded-lg bg-muted/30 p-3"
        />
      ) : null}
    </div>
  )
}

// Dashboard panel of the latest firings, each with the quick silence
// action and an expandable "what changed" block. Renders nothing when
// alerting is unavailable or nothing has fired: an empty panel on every
// dashboard load is noise.
export function RecentAlertsCard() {
  const { data, isError } = useAlertHistory({ event: 'fired', limit: ROW_CAP })
  if (isError || !data || data.length === 0) {
    return null
  }

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle className="flex items-center gap-1.5 text-sm">
          <BellRingingIcon
            className="size-4 text-muted-foreground"
            aria-hidden="true"
          />
          Recent alerts
          <Link
            to="/alerts"
            className="ml-auto text-xs font-normal text-muted-foreground hover:text-foreground"
          >
            History
          </Link>
        </CardTitle>
      </CardHeader>
      <CardContent className="divide-y divide-border">
        {data.map((entry) => (
          <AlertRow key={entry.id} entry={entry} />
        ))}
      </CardContent>
    </Card>
  )
}
