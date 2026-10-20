import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import type { DnsPlanResult } from '../../types/dns'

const ACTION_VARIANT = {
  create: 'success',
  update: 'warning',
  delete: 'destructive',
  unchanged: 'muted',
} as const

/** PlanView shows a diff before anything is applied: changes, conflicts, warnings and parse errors. */
export function PlanView({ result }: { result: DnsPlanResult }) {
  const { t } = useTranslation('dns')
  const changes = result.plan.changes.filter((c) => c.action !== 'unchanged')
  const s = result.plan.summary
  return (
    <div className="space-y-2">
      <p className="text-sm text-muted-foreground">
        {t('plan.summary', {
          create: s.create ?? 0,
          update: s.update ?? 0,
          delete: s.delete ?? 0,
          unchanged: s.unchanged ?? 0,
        })}
      </p>
      {changes.length > 0 ? (
        <ul className="max-h-64 space-y-1 overflow-auto rounded-md border border-border p-2 text-sm">
          {changes.map((c) => (
            <li
              key={`${c.action}|${c.set.name}|${c.set.type}|${c.set.set_identifier ?? ''}`}
            >
              <div className="flex items-start gap-2">
                <Badge variant={ACTION_VARIANT[c.action]}>
                  {t(`plan.action.${c.action}`)}
                </Badge>
                <span className="font-mono">
                  {c.set.name} {c.set.type}
                </span>
                <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
                  {c.set.values.join(' | ')}
                </span>
              </div>
              {c.issues?.map((is) => (
                <p
                  key={is.code}
                  className={
                    is.severity === 'error'
                      ? 'pl-2 text-xs text-destructive'
                      : 'pl-2 text-xs text-muted-foreground'
                  }
                >
                  {is.message}
                </p>
              ))}
            </li>
          ))}
        </ul>
      ) : null}
      {result.warnings?.map((w) => (
        <p key={w} className="text-xs text-muted-foreground">
          {w}
        </p>
      ))}
      {result.errors?.map((e) => (
        <p key={e} className="text-xs text-destructive">
          {e}
        </p>
      ))}
      {result.plan.blocked ? (
        <p className="text-sm text-destructive" role="alert">
          {t('plan.blocked')}
        </p>
      ) : null}
    </div>
  )
}
