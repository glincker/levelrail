import { useTranslation } from 'react-i18next'
import { RestoreGlassIcon } from '@thesvg/ui-react'
import { Card, CardContent } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { RelativeTime } from '@/components/kit'
import { useFirewallAudit, type FwAuditEntry } from '../../queries/firewallEdge'
import { LoadError } from './LoadError'
import { useFirewallLabels } from './useFirewallLabels'
import { VirtualRows } from './VirtualRows'

const SHOWN_DETAILS = [
  'name',
  'ip',
  'scope',
  'action',
  'until',
  'bot_mode',
  'ai_bot_mode',
]

function detailText(d: FwAuditEntry['detail']): string {
  if (!d) return ''
  return SHOWN_DETAILS.filter((k) => d[k])
    .map((k) => `${k}=${d[k]}`)
    .join('  ')
}

/** AuditView lists every firewall change with who made it, newest first. */
export function AuditView() {
  const { t } = useTranslation('firewall')
  const labels = useFirewallLabels()
  const { data, error, refetch } = useFirewallAudit()

  if (error) return <LoadError error={error} onRetry={() => void refetch()} />

  return (
    <Card>
      <CardContent className="px-2">
        {!data ? (
          <div aria-hidden="true" className="space-y-2 px-2">
            {Array.from({ length: 6 }, (_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        ) : data.length === 0 ? (
          <EmptyState
            compact
            icon={<RestoreGlassIcon className="size-5.5" />}
            title={t('audit.emptyTitle')}
            description={t('audit.emptyHelp')}
          />
        ) : (
          <VirtualRows
            items={data}
            rowHeight={56}
            label={t('audit.title')}
            getKey={(e) => e.id}
            renderRow={(e) => (
              <div className="flex flex-wrap items-baseline gap-x-4 gap-y-0.5 px-2 py-2 text-sm">
                <span className="w-24 shrink-0 text-xs text-muted-foreground">
                  <RelativeTime at={e.created_at} />
                </span>
                <span className="min-w-0 basis-48 font-medium">
                  {labels.auditAction[e.action] ?? e.action}
                </span>
                <span className="text-muted-foreground">{e.actor_name}</span>
                <span
                  className="min-w-0 flex-1 basis-56 truncate font-mono text-xs text-muted-foreground"
                  title={detailText(e.detail)}
                >
                  {detailText(e.detail)}
                </span>
              </div>
            )}
          />
        )}
      </CardContent>
    </Card>
  )
}
