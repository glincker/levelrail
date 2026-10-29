import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { GaugeGlassIcon } from '@thesvg/ui-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import {
  useAdvisorFindings,
  useResourceAdvisorEnabled,
} from '../../queries/resourceAdvisor'
import { splitPending } from '../../lib/resourceAdvisor'
import { CostComparison } from './CostComparison'
import { FindingCard } from './FindingCard'
import { LimitSuggestion } from './LimitSuggestion'

function AdvisorFindingsFor({
  resource,
  name,
}: {
  resource: string
  name: string
}) {
  const { t } = useTranslation('resourceAdvisor')
  const { data, isPending, isError, error } = useAdvisorFindings(resource)
  if (isPending) return null
  if (isError) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('error', { message: error.message })}
      </p>
    )
  }
  const { advice, pending } = splitPending(data.findings)
  if (advice.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {pending[0]?.why ?? t('panel.none', { name })}
      </p>
    )
  }
  return (
    <div className="space-y-3">
      {advice.map((f) => (
        <FindingCard key={f.id} finding={f} />
      ))}
    </div>
  )
}

type Props =
  | { appName: string; databaseName?: never }
  | { appName?: never; databaseName: string }

// One panel for everything about sizing a resource: advisor findings when
// the beta is on, the plain limit suggestion otherwise, and the cost
// comparison folded away underneath.
export function OptimisePanel(props: Props) {
  const { t } = useTranslation('resourceAdvisor')
  const advisorOn = useResourceAdvisorEnabled()
  const name = props.appName ?? props.databaseName ?? ''
  const resource = props.appName
    ? `app:${props.appName}`
    : `database:${props.databaseName}`
  return (
    <Card data-testid="optimise-panel">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GaugeGlassIcon className="size-4.5" />
          {t('panel.title')}
          {advisorOn ? (
            <Badge variant="outline" title={t('beta.hint')}>
              {t('beta.label')}
            </Badge>
          ) : null}
        </CardTitle>
        <p className="text-sm text-muted-foreground">
          {t('panel.description', { name })}
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        {advisorOn ? (
          <>
            <AdvisorFindingsFor resource={resource} name={name} />
            <Link
              to="/resource-advisor"
              className="text-xs text-muted-foreground underline-offset-4 hover:underline"
            >
              {t('panel.openAdvisor')}
            </Link>
          </>
        ) : (
          <LimitSuggestion
            appName={props.appName}
            databaseName={props.databaseName}
          />
        )}
        {props.appName ? <CostComparison appName={props.appName} /> : null}
      </CardContent>
    </Card>
  )
}
