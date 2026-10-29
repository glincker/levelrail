import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { DatabaseGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '@/hooks/useIsRoot'
import {
  useCancelRestore,
  useCheckIntegrity,
  useIntegrity,
  useStageRestore,
  type IntegrityCandidate,
} from '../../queries/dbIntegrity'
import {
  useRestartControlPlane,
  useSystemRestartInfo,
} from '../../queries/systemRestart'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import { StepUpDialog } from '../env/StepUpDialog'

export function IntegrityCard() {
  const { t } = useTranslation('settings')
  const isRoot = useIsRoot()
  const { data } = useIntegrity()
  const check = useCheckIntegrity()
  const stage = useStageRestore()
  const cancel = useCancelRestore()
  const restart = useRestartControlPlane()
  const { data: restartInfo } = useSystemRestartInfo()
  const [target, setTarget] = useState<IntegrityCandidate | null>(null)
  const gate = useStepUpGate((e) =>
    toast.add({
      title: e instanceof Error ? e.message : String(e),
      type: 'error',
    }),
  )

  if (!data) return null

  const runCheck = (full: boolean) =>
    check.mutate(full, {
      onSuccess: (s) =>
        toast.add({
          title: t(
            s.healthy
              ? 'controlPlaneHealth.integrity.checkOk'
              : 'controlPlaneHealth.integrity.checkBad',
          ),
          type: s.healthy ? 'success' : 'error',
        }),
      onError: (e) =>
        toast.add({
          title: t('controlPlaneHealth.fixFailed'),
          description: e.message,
          type: 'error',
        }),
    })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          <DatabaseGlassIcon size="1.4em" />
          {t('controlPlaneHealth.integrity.title')}
          <Badge variant={data.status.healthy ? 'success' : 'destructive'}>
            {t(
              data.status.healthy
                ? 'controlPlaneHealth.integrity.ok'
                : 'controlPlaneHealth.integrity.damaged',
            )}
          </Badge>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">
          {data.status.checks === 0
            ? t('controlPlaneHealth.integrity.notYet')
            : t('controlPlaneHealth.integrity.last', {
                when: new Date(data.status.checked_at).toLocaleString(),
                mode: t(
                  `controlPlaneHealth.integrity.mode.${data.status.mode}`,
                ),
              })}
        </p>
        {data.status.error ? (
          <Banner variant="critical" title={data.status.error} />
        ) : null}
        {data.pending ? (
          <Banner
            variant="warning"
            title={t('controlPlaneHealth.integrity.pending', {
              name: data.pending.name,
            })}
            action={
              isRoot ? (
                <div className="flex flex-wrap gap-2">
                  {restartInfo?.supported ? (
                    <Button
                      size="sm"
                      disabled={restart.isPending}
                      onClick={() => restart.mutate()}
                    >
                      {t('controlPlaneHealth.integrity.restartNow')}
                    </Button>
                  ) : null}
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={cancel.isPending}
                    onClick={() => cancel.mutate()}
                  >
                    {t('controlPlaneHealth.integrity.cancelRestore')}
                  </Button>
                </div>
              ) : undefined
            }
          />
        ) : null}
        {isRoot ? (
          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              variant="outline"
              disabled={check.isPending}
              onClick={() => runCheck(false)}
            >
              {t('controlPlaneHealth.integrity.checkQuick')}
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={check.isPending}
              onClick={() => runCheck(true)}
            >
              {t('controlPlaneHealth.integrity.checkFull')}
            </Button>
          </div>
        ) : null}
        <div className="space-y-2">
          <p className="text-sm font-medium">
            {t('controlPlaneHealth.integrity.snapshots')}
          </p>
          {data.candidates.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {t('controlPlaneHealth.integrity.noSnapshots')}
            </p>
          ) : (
            data.candidates.map((c) => (
              <div
                key={c.name}
                className="flex flex-wrap items-center justify-between gap-2 text-sm"
              >
                <span className="min-w-0 truncate font-mono text-xs">
                  {c.name}
                </span>
                <span className="flex items-center gap-2">
                  <Badge variant={c.verified ? 'success' : 'warning'}>
                    {t(
                      c.verified
                        ? 'controlPlaneHealth.integrity.verified'
                        : 'controlPlaneHealth.integrity.unverified',
                    )}
                  </Badge>
                  {isRoot ? (
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => setTarget(c)}
                    >
                      {t('controlPlaneHealth.integrity.restore')}
                    </Button>
                  ) : null}
                </span>
              </div>
            ))
          )}
        </div>
        <Collapsible>
          <CollapsibleTrigger className="text-xs text-muted-foreground underline">
            {t('controlPlaneHealth.fixViaCli')}
          </CollapsibleTrigger>
          <CollapsiblePanel>
            <code className="mt-1 block text-xs">
              system integrity --restore latest
            </code>
          </CollapsiblePanel>
        </Collapsible>
      </CardContent>
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(open) => {
          if (!open) setTarget(null)
        }}
        title={t('controlPlaneHealth.integrity.confirmTitle', {
          name: target?.name ?? '',
        })}
        description={t('controlPlaneHealth.integrity.confirmDescription')}
        consequences={[
          t('controlPlaneHealth.integrity.consequenceKept'),
          t('controlPlaneHealth.integrity.consequenceRestart'),
        ]}
        confirmLabel={t('controlPlaneHealth.integrity.restore')}
        tone="destructive"
        pending={stage.isPending}
        error={stage.error?.message ?? null}
        onConfirm={() => {
          if (!target) return
          const name = target.name
          void gate.guard(
            { action: 'manage', app: 'integrity-restore' },
            async () => {
              await stage.mutateAsync(name)
              toast.add({
                title: t('controlPlaneHealth.integrity.staged'),
                type: 'success',
              })
              setTarget(null)
            },
          )
        }}
      />
      <StepUpDialog
        scope={gate.pending?.scope ?? null}
        onVerified={() => {
          void gate.onVerified()
        }}
        onCancel={gate.cancel}
      />
    </Card>
  )
}
