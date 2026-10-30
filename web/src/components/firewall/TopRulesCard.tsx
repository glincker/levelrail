import { useTranslation } from 'react-i18next'
import { ListChecksGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Sparkline, StatusPill } from '@/components/kit'
import type { FwRule } from '../../queries/firewallEdge'
import { ACTION_TONE, formatCount } from './firewallFormat'
import { useFirewallLabels } from './useFirewallLabels'

const MAX_ROWS = 6

/** TopRulesCard ranks rules by hits so the ones doing the work are visible at a glance. */
export function TopRulesCard({
  rules,
  onOpenRules,
  onNewRule,
}: {
  rules: FwRule[]
  onOpenRules: () => void
  onNewRule: () => void
}) {
  const { t } = useTranslation('firewall')
  const labels = useFirewallLabels()
  const ranked = [...rules]
    .sort((a, b) => b.hits.total - a.hits.total)
    .slice(0, MAX_ROWS)
  return (
    <Card className="min-w-0">
      <CardHeader className="border-b">
        <CardTitle>{t('rules.title')}</CardTitle>
        <div data-slot="card-action">
          <Button type="button" size="xs" variant="ghost" onClick={onOpenRules}>
            {t('rules.manage')}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="px-2">
        {ranked.length === 0 ? (
          <EmptyState
            compact
            icon={<ListChecksGlassIcon className="size-6" />}
            title={t('rules.emptyTitle')}
            description={t('rules.emptyHelp')}
            action={
              <Button type="button" size="sm" onClick={onNewRule}>
                {t('rules.new')}
              </Button>
            }
          />
        ) : (
          <ul className="divide-y divide-border">
            {ranked.map((r) => (
              <li
                key={r.id}
                className="flex items-center gap-3 px-2 py-1.5 text-sm"
              >
                <span className="min-w-0 flex-1 truncate" title={r.name}>
                  {r.name}
                </span>
                <StatusPill
                  tone={ACTION_TONE[r.action]}
                  label={labels.action[r.action]}
                  size="sm"
                />
                <span className="w-12 text-right font-medium tabular-nums">
                  {formatCount(r.hits.total)}
                </span>
                <Sparkline
                  values={r.hits.sparkline}
                  width={64}
                  height={20}
                  ariaLabel={t('rules.hitsSpark', { name: r.name })}
                />
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
