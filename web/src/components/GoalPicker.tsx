import { useTranslation } from 'react-i18next'
import {
  AlarmClockGlassIcon,
  ApiEndpointGlassIcon,
  DatabaseGlassIcon,
  GlobeGlassIcon,
  LayoutTemplateGlassIcon,
  ServerGlassIcon,
} from '@thesvg/ui-react'
import { goalsForScope, type GoalId } from '@/lib/createGoals'

const GOAL_ICONS: Record<GoalId, React.ReactNode> = {
  website: <GlobeGlassIcon aria-hidden="true" className="size-5" />,
  api: <ApiEndpointGlassIcon aria-hidden="true" className="size-5" />,
  database: <DatabaseGlassIcon aria-hidden="true" className="size-5" />,
  'static-site': (
    <LayoutTemplateGlassIcon aria-hidden="true" className="size-5" />
  ),
  'scheduled-job': (
    <AlarmClockGlassIcon aria-hidden="true" className="size-5" />
  ),
  'self-hosted': <ServerGlassIcon aria-hidden="true" className="size-5" />,
}

// One compact row of goals above the full catalog; the catalog stays
// one scroll away for anything the goals do not cover.
export function GoalPicker({
  scope,
  onSelect,
}: {
  scope?: 'applications' | 'databases'
  onSelect: (id: GoalId) => void
}) {
  const { t } = useTranslation('databases')
  const goals = goalsForScope(scope)
  return (
    <section aria-label={t('wizard.goals.title')} className="space-y-2">
      <h3 className="text-xs font-medium text-muted-foreground">
        {t('wizard.goals.title')}
      </h3>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6">
        {goals.map((goal) => (
          <button
            key={goal.id}
            type="button"
            onClick={() => {
              onSelect(goal.id)
            }}
            title={t(`wizard.goals.items.${goal.id}.hint`)}
            className="flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-2 text-left text-sm font-medium transition-colors hover:border-primary/40 hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none motion-reduce:transition-none"
          >
            <span className="shrink-0 text-muted-foreground">
              {GOAL_ICONS[goal.id]}
            </span>
            <span className="min-w-0 leading-tight">
              {t(`wizard.goals.items.${goal.id}.label`)}
            </span>
          </button>
        ))}
      </div>
    </section>
  )
}
