import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import type { PolicyCheckResult } from '../../queries/deployPolicies'

export function PolicyCheckResults({ result }: { result: PolicyCheckResult }) {
  const { t } = useTranslation('deployPolicies')
  return (
    <div className="space-y-3" aria-live="polite">
      <p className="text-sm">
        {t('result.summary', {
          checked: result.checked,
          failing: result.failing,
          warning: result.warning,
        })}
      </p>
      {result.apps.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('result.clean')}</p>
      ) : null}
      {result.apps.map((a) => (
        <Card key={a.app}>
          <CardContent className="space-y-2">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium">{a.app}</span>
              <Badge variant={a.would_fail ? 'destructive' : 'warning'}>
                {a.would_fail ? t('result.wouldFail') : t('result.warns')}
              </Badge>
            </div>
            <ul className="space-y-2">
              {a.violations.map((v, i) => (
                <li key={`${v.code}-${i}`} className="text-sm">
                  <p>
                    {v.message}{' '}
                    <code className="text-xs text-muted-foreground">
                      {v.code}
                    </code>
                  </p>
                  <p className="text-muted-foreground">
                    {t('result.next')}: {v.next_action}
                  </p>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
