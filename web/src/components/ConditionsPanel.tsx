import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import type { ReconcileCondition } from '../types/deploy'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { RelativeTime } from '@/components/kit'
import { TONE, type Tone } from '@/components/kit/tone'
import { cn } from '@/lib/utils'
import { humanizeCondition, type ConditionOutcome } from '../lib/conditionCopy'

const OUTCOME_TONE: Record<ConditionOutcome, Tone> = {
  ok: 'success',
  failing: 'danger',
  pending: 'neutral',
}

const CREDENTIALS_NOT_CONFIGURED = 'CredentialsNotConfigured'

// Current reconcile status (latest condition per type), not a history log.
// Raw condition vocabulary is humanised; the exact string stays one click
// away under "Technical details" for support.
export function ConditionsPanel({
  conditions,
}: {
  conditions: ReconcileCondition[]
}) {
  const { t } = useTranslation('databases')

  if (conditions.length === 0) {
    return (
      <Card>
        <CardContent className="text-sm text-muted-foreground">
          {t('conditions.empty')}
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('conditions.title')}</CardTitle>
      </CardHeader>
      <CardContent>
        <ul className="-mx-4 divide-y divide-border">
          {conditions.map((c) => {
            const human = humanizeCondition(c)
            const tone = TONE[OUTCOME_TONE[human.outcome]]
            return (
              <li
                key={c.Type}
                className="flex items-start gap-3 px-4 py-3 first:pt-0 last:pb-0"
              >
                <span
                  aria-hidden="true"
                  className={cn(
                    'mt-1.5 size-2 shrink-0 rounded-full',
                    tone.solid,
                  )}
                />
                <div className="min-w-0 flex-1">
                  <p className="flex flex-wrap items-baseline gap-x-2 text-sm">
                    <span className="font-medium text-foreground">
                      {human.title}
                    </span>
                    <span className={cn('text-xs font-medium', tone.text)}>
                      {t(`conditions.${human.outcome}`)}
                    </span>
                  </p>
                  {human.detail ? (
                    <p className="text-sm text-muted-foreground">
                      {human.detail}
                    </p>
                  ) : null}
                  {c.Reason === CREDENTIALS_NOT_CONFIGURED ? (
                    <div
                      className={cn(
                        'mt-1 flex items-start gap-2 rounded-lg border p-2.5 text-sm',
                        TONE.warning.soft,
                        TONE.warning.border,
                        TONE.warning.text,
                      )}
                    >
                      <TriangleAlertGlassIcon
                        className="mt-0.5 size-5 shrink-0 text-tone-warning"
                        aria-hidden="true"
                      />
                      <p>
                        {t('conditions.credentialsHelp')}{' '}
                        <Link
                          to="/settings/general"
                          className="underline underline-offset-2"
                        >
                          {t('conditions.credentialsLink')}
                        </Link>
                      </p>
                    </div>
                  ) : null}
                  <p className="mt-1 text-xs text-muted-foreground/70">
                    {t('conditions.updated')}{' '}
                    <RelativeTime at={c.LastTransitionTime} />
                  </p>
                  <details className="mt-1 text-xs text-muted-foreground">
                    <summary className="cursor-pointer select-none">
                      {t('conditions.details')}
                    </summary>
                    <p className="mt-1 font-mono break-words">{human.raw}</p>
                    {c.Message ? (
                      <p className="mt-1 break-words">{c.Message}</p>
                    ) : null}
                  </details>
                </div>
              </li>
            )
          })}
        </ul>
      </CardContent>
    </Card>
  )
}
