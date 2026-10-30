import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PencilGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { useIsRoot } from '../../hooks/useIsRoot'
import { useLogBudget, useLogUsage } from '../../queries/logBudgets'
import { TrimLogsButton } from './TrimLogsButton'
import { LogBudgetEditor } from './LogBudgetEditor'

/** AppLogBudgetField shows and edits one app's log budget on its Logs page. */
export function AppLogBudgetField({ app }: { app: string }) {
  const { t } = useTranslation('logBudgets')
  const isRoot = useIsRoot()
  const budget = useLogBudget(app)
  const usage = useLogUsage()
  const [editing, setEditing] = useState(false)
  if (!budget.data) return null
  const { max_mb: maxMb, default_mb: defaultMb } = budget.data
  const effective = maxMb > 0 ? maxMb : defaultMb
  const percent = usage.data?.apps.find((a) => a.app === app)?.percent ?? null
  const over = percent !== null && percent > 100
  return (
    <div className="flex items-center gap-2 text-sm text-muted-foreground">
      <span>{t('budgetField')}</span>
      {editing ? (
        <LogBudgetEditor
          app={app}
          initialMb={maxMb}
          onDone={() => setEditing(false)}
        />
      ) : (
        <>
          <span className="tabular-nums text-foreground">
            {effective > 0 ? effective : t('unlimited')}
          </span>
          {isRoot && (
            <Button
              type="button"
              size="icon-sm"
              variant="ghost"
              aria-label={t('editFor', { app })}
              onClick={() => setEditing(true)}
            >
              <PencilGlassIcon size="1.4em" />
            </Button>
          )}
        </>
      )}
      {over && (
        <>
          <span className="text-tone-warning">{t('overBudget')}</span>
          {isRoot && <TrimLogsButton app={app} />}
        </>
      )}
    </div>
  )
}
