import { Link } from '@tanstack/react-router'
import { BellRingingIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useAlertHistory } from '../queries/alertNoise'
import { SilenceRuleMenu } from './SilenceRuleMenu'

const ROW_CAP = 5

// Dashboard panel of the latest firings, each with the quick silence
// action. Renders nothing when alerting is unavailable or nothing has
// fired: an empty panel on every dashboard load is noise.
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
          <div
            key={entry.id}
            className="flex flex-wrap items-center justify-between gap-2 py-2 first:pt-0 last:pb-0"
          >
            <div className="min-w-0">
              <p className="truncate text-sm font-medium text-foreground">
                {entry.rule_name}
              </p>
              <p className="text-xs text-muted-foreground">
                {entry.app ?? 'platform'} &middot;{' '}
                {new Date(entry.at).toLocaleString()}
              </p>
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
        ))}
      </CardContent>
    </Card>
  )
}
