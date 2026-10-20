import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  hubReceiptUrl,
  useApplyHubSession,
  useSetHubStep,
  type HubItemStatus,
  type HubSession,
} from '../queries/migrationHub'
import { formatBytes } from './migrationHubFormat'

const statusVariant: Record<
  HubItemStatus,
  'muted' | 'warning' | 'success' | 'destructive'
> = {
  pending: 'muted',
  copying: 'warning',
  verified: 'success',
  failed: 'destructive',
}

export function MigrationHubCopy({
  session,
  verifyView,
  onNext,
}: {
  session: HubSession
  verifyView: boolean
  onNext: () => void
}) {
  const { t } = useTranslation('migration')
  const apply = useApplyHubSession()
  const setStep = useSetHubStep()
  const [password, setPassword] = useState('')
  const selected = session.items.filter((i) => i.selected)
  const sum = session.summary
  const done = sum.verified + sum.failed

  const started =
    session.running ||
    selected.some((i) => i.status !== 'pending') ||
    ['copy', 'verify', 'cutover'].includes(session.step)
  if (selected.length === 0 || !started) {
    return (
      <p className="text-sm text-muted-foreground">{t('hub.copy.empty')}</p>
    )
  }

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-base font-semibold">
          {verifyView ? t('hub.verify.title') : t('hub.copy.title')}
        </h3>
        <p className="text-sm text-muted-foreground">
          {verifyView ? t('hub.verify.description') : t('hub.copy.description')}
        </p>
        <p className="text-sm" aria-live="polite">
          {session.running
            ? t('hub.copy.running', { done, total: sum.selected })
            : t('hub.copy.idle')}
        </p>
      </div>
      <ul className="space-y-2">
        {selected.map((i) => {
          const bad = (i.table_counts ?? []).filter((x) => !x.ok)
          return (
            <li key={i.source_db} className="rounded-md border p-3 text-sm">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{i.source_db}</span>
                <span className="text-xs text-muted-foreground">
                  {i.target_name}, {formatBytes(i.size_bytes)}
                </span>
                <Badge variant={statusVariant[i.status]}>
                  {t(`hub.copy.status.${i.status}`)}
                </Badge>
                {i.status === 'verified' ? (
                  <span className="text-xs text-muted-foreground">
                    {t('hub.verify.tables', { count: i.checked })}
                  </span>
                ) : null}
              </div>
              {i.reason ? (
                <p className="mt-2 break-words whitespace-pre-wrap text-destructive">
                  {i.reason}
                </p>
              ) : null}
              {bad.map((x) => (
                <p key={x.name} className="mt-1 font-mono text-xs">
                  {t('hub.verify.differs', {
                    name: x.name,
                    source: x.source,
                    target: x.target,
                  })}
                </p>
              ))}
              {verifyView && i.status === 'verified' && i.table_counts ? (
                <details className="mt-2">
                  <summary className="cursor-pointer text-xs text-muted-foreground">
                    {t('hub.verify.tables', { count: i.table_counts.length })}
                  </summary>
                  <ul className="mt-1 space-y-0.5 font-mono text-xs">
                    {i.table_counts.map((x) => (
                      <li key={x.name}>
                        {x.name}: {x.source} / {x.target}
                      </li>
                    ))}
                  </ul>
                </details>
              ) : null}
            </li>
          )
        })}
      </ul>

      {sum.failed > 0 && !session.running ? (
        <div className="space-y-2">
          {!session.password_held ? (
            <div className="max-w-sm space-y-1.5">
              <Label htmlFor="hub-retry-pw">
                {t('hub.preflight.passwordTitle')}
              </Label>
              <Input
                id="hub-retry-pw"
                type="password"
                autoComplete="off"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
          ) : null}
          {apply.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{apply.error.message}</AlertDescription>
            </Alert>
          ) : null}
          <Button
            variant="outline"
            disabled={apply.isPending || (!session.password_held && !password)}
            onClick={() => apply.mutate({ id: session.id, password })}
          >
            {t('hub.copy.retry')}
          </Button>
        </div>
      ) : null}

      {verifyView ? (
        <div className="space-y-2">
          <p className="text-sm text-muted-foreground">
            {t('hub.verify.source')}. {t('hub.verify.receiptHelp')}
          </p>
          <div className="flex flex-wrap gap-2">
            {sum.verified > 0 ? (
              <Button
                variant="outline"
                render={<a href={hubReceiptUrl(session.id)} download />}
              >
                {t('hub.verify.receipt')}
              </Button>
            ) : (
              <p className="text-sm text-muted-foreground">
                {t('hub.verify.noneVerified')}
              </p>
            )}
            <Button
              disabled={sum.verified === 0 || session.running}
              onClick={() =>
                setStep.mutate(
                  { id: session.id, step: 'cutover' },
                  { onSuccess: onNext },
                )
              }
            >
              {t('hub.verify.next')}
            </Button>
          </div>
        </div>
      ) : (
        <Button
          disabled={sum.verified === 0 || session.running}
          onClick={() =>
            setStep.mutate(
              { id: session.id, step: 'verify' },
              { onSuccess: onNext },
            )
          }
        >
          {t('hub.copy.continue')}
        </Button>
      )}
    </div>
  )
}
