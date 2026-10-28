import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ArrowFatLinesUpIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'

interface TemplateUpgradePlan {
  has_changes: boolean
  risk: 'none' | 'low' | 'medium' | 'high'
  blockers: ReadonlyArray<{ code: string }>
}

// 404 means the app is not a template app, which is not an error here.
async function fetchPlan(appName: string): Promise<TemplateUpgradePlan | null> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/template-upgrade`,
  )
  if (!res.ok) return null
  return (await res.json()) as TemplateUpgradePlan
}

/** Shows when the catalog template an app came from has moved on. */
export function TemplateUpgradeBadge({
  appName,
}: Readonly<{ appName: string }>) {
  const { t } = useTranslation('deploys')
  const { data } = useQuery({
    queryKey: ['apps', appName, 'template-upgrade'],
    queryFn: () => fetchPlan(appName),
    retry: false,
    staleTime: 60_000,
  })
  if (!data?.has_changes) return null
  return (
    <div className="flex items-center gap-2">
      <Badge variant="info" className="gap-1">
        <ArrowFatLinesUpIcon className="size-3.5" aria-hidden="true" />
        {t('templateUpgrade.badge')}
      </Badge>
      <span className="text-xs text-muted-foreground">
        {t(`templateUpgrade.risk.${data.risk}`)}
        {data.blockers.length > 0 ? ` · ${t('templateUpgrade.blocked')}` : ''}
      </span>
    </div>
  )
}
