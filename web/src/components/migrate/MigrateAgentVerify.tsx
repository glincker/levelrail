import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useVerifyAgent,
  type AgentComparison,
  type AgentVerifyResult,
} from '../../queries/migrateAgent'
import {
  useMigrateChecklists,
  useStepChecklist,
} from '../../queries/migrateWizard'

function ComparisonRow({ c }: { c: AgentComparison }) {
  const { t } = useTranslation('migrate')
  return (
    <li className="flex flex-wrap items-center gap-2 py-2 text-sm">
      <span className="font-medium">{c.name}</span>
      <Badge variant={c.status === 'match' ? 'success' : 'warning'}>
        {t(`agent.status.${c.status}`)}
      </Badge>
      <span className="text-muted-foreground">
        {t('agent.compare', {
          sf: c.source_files,
          sb: c.source_bytes,
          tf: c.target_files,
          tb: c.target_bytes,
        })}
      </span>
      {c.target_from === 'operator-supplied' ? (
        <Badge variant="warning">{t('agent.operatorSupplied')}</Badge>
      ) : null}
      {c.detail ? (
        <span className="text-muted-foreground">{c.detail}</span>
      ) : null}
    </li>
  )
}

function VerifyResult({
  app,
  result,
}: {
  app: string
  result: AgentVerifyResult
}) {
  const { t } = useTranslation('migrate')
  const step = useStepChecklist()
  const undo = () => {
    step.mutate(
      { app, step: 'data_verified', req: { action: 'reset' } },
      {
        onError: (e) =>
          toast.add({
            title: e instanceof Error ? e.message : t('cutover.stepError'),
            type: 'error',
          }),
      },
    )
  }
  return (
    <div className="space-y-2">
      <ul className="divide-y">
        {result.comparisons.map((c) => (
          <ComparisonRow key={c.name} c={c} />
        ))}
      </ul>
      {result.advance.advanced ? (
        <Banner variant="success" title={t('agent.advanced')}>
          <div className="flex flex-wrap items-center gap-2">
            <span>{t('agent.advancedHint')}</span>
            <Button
              size="sm"
              variant="outline"
              disabled={step.isPending || step.isSuccess}
              onClick={undo}
            >
              {step.isSuccess ? t('agent.undone') : t('agent.undo')}
            </Button>
          </div>
        </Banner>
      ) : (
        <Banner variant="warning" title={t('agent.notAdvanced')}>
          {result.advance.reason}
        </Banner>
      )}
      {result.databases_on_source > 0 ? (
        <Banner variant="info" title={t('agent.databasesManual')}>
          {t('agent.databasesManualHint', {
            count: result.databases_on_source,
          })}
        </Banner>
      ) : null}
    </div>
  )
}

export function MigrateAgentVerify({ sessionId }: { sessionId: string }) {
  const { t } = useTranslation('migrate')
  const checklists = useMigrateChecklists()
  const verify = useVerifyAgent()
  const [ackEmpty, setAckEmpty] = useState(false)
  const [shown, setShown] = useState<{
    app: string
    result: AgentVerifyResult
  } | null>(null)
  const apps = (checklists.data ?? []).filter(
    (c) => (c.volumes ?? []).length > 0,
  )

  function run(app: string) {
    setShown(null)
    verify.mutate(
      { id: sessionId, app, ackEmpty },
      {
        onSuccess: (result) => setShown({ app, result }),
        onError: (e) =>
          toast.add({
            title: e instanceof Error ? e.message : t('agent.verifyError'),
            type: 'error',
          }),
      },
    )
  }

  if (apps.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('agent.noVolumes')}</p>
    )
  }
  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">{t('agent.verifyHint')}</p>
      <label className="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={ackEmpty}
          onChange={(e) => setAckEmpty(e.target.checked)}
        />
        <span>{t('agent.ackEmpty')}</span>
      </label>
      <ul className="divide-y">
        {apps.map((c) => (
          <li key={c.app} className="flex items-center gap-3 py-2">
            <span className="text-sm font-medium">{c.app}</span>
            <span className="text-sm text-muted-foreground">
              {t('agent.volumeCount', { count: c.volumes?.length ?? 0 })}
            </span>
            <Button
              size="sm"
              className="ml-auto"
              disabled={verify.isPending}
              onClick={() => run(c.app)}
            >
              {verify.isPending && verify.variables?.app === c.app
                ? t('agent.verifying')
                : t('agent.verify')}
            </Button>
          </li>
        ))}
      </ul>
      {shown ? <VerifyResult app={shown.app} result={shown.result} /> : null}
    </div>
  )
}
