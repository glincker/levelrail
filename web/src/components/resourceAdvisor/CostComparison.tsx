import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon, CoinsGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { HelpLink } from '../HelpLink'
import { useCostEstimate } from '../../queries/costEstimate'
import type { CostBasis } from '../../types/costEstimate'

function useBasis() {
  const { t } = useTranslation('resourceAdvisor')
  return (b: CostBasis) =>
    b === 'declared'
      ? t('cost.declared')
      : b === 'observed'
        ? t('cost.observed')
        : t('cost.none')
}

function CostBody({ appName }: { appName: string }) {
  const { t } = useTranslation('resourceAdvisor')
  const basis = useBasis()
  const { data, isPending, isError } = useCostEstimate(appName)
  if (isPending) return null
  if (isError)
    return <p className="text-sm text-muted-foreground">{t('cost.error')}</p>
  const hasSize =
    data.cpu_basis !== 'unavailable' || data.memory_basis !== 'unavailable'
  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        {hasSize
          ? t('cost.basis', {
              cpu: data.vcpu_cores.toFixed(2),
              cpuBasis: basis(data.cpu_basis),
              memory: data.memory_gib.toFixed(2),
              memoryBasis: basis(data.memory_basis),
            })
          : t('cost.noSize')}
      </p>
      <ul className="space-y-1.5">
        {data.providers.map((p) => (
          <li
            key={p.key}
            className="flex items-center justify-between gap-3 text-sm"
          >
            <span className="min-w-0">
              <span className="text-foreground">{p.label}</span>
              <span className="block text-xs text-muted-foreground">
                {t('cost.split', {
                  cpu: p.cpu_cost_usd.toFixed(2),
                  memory: p.memory_cost_usd.toFixed(2),
                })}
              </span>
            </span>
            <span className="flex items-center gap-2 tabular-nums">
              {p.minimum_applied ? (
                <Badge variant="muted">{t('cost.minimum')}</Badge>
              ) : null}
              {t('cost.perMonth', { amount: p.total_usd.toFixed(2) })}
            </span>
          </li>
        ))}
      </ul>
      <p className="text-xs text-muted-foreground">{t('cost.note')}</p>
    </div>
  )
}

// Collapsed by default: illustrative prices are context, not the main point
// of the Optimise panel. The estimate is fetched only once it is opened.
export function CostComparison({ appName }: { appName: string }) {
  const { t } = useTranslation('resourceAdvisor')
  return (
    <Collapsible className="border-t border-border pt-3">
      <div className="flex items-center gap-2">
        <CollapsibleTrigger className="group flex w-auto items-center gap-2 text-sm font-medium text-muted-foreground hover:text-foreground">
          <ChevronDownGlassIcon className="size-3.5 transition-transform group-data-[panel-open]:rotate-180" />
          <CoinsGlassIcon className="size-4.5" />
          {t('cost.title')}
        </CollapsibleTrigger>
        <HelpLink path="/cost-estimate" label={t('cost.help')} />
      </div>
      <CollapsiblePanel className="pt-3">
        <CostBody appName={appName} />
      </CollapsiblePanel>
    </Collapsible>
  )
}
