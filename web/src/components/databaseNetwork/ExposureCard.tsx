import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GlobeGlassIcon } from '@thesvg/ui-react'
import { useQueryClient } from '@tanstack/react-query'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { CodeBlock } from '@/components/ui/code-block'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { SegmentedControl } from '@/components/ui/segmented-control'
import { toast } from '@/components/ui/toast'
import { tunnelCommand } from '@/lib/connectSnippets'
import type { DatabaseNetwork } from '../../types/databaseAccess'
import {
  LEVELS,
  currentBind,
  currentLevel,
  needsTypedConfirm,
  publishDisabledReason,
  type BindChoice,
  type ExposureLevel,
} from './exposureLevel'
import {
  databaseAccessKeys,
  useMakePrivate,
} from '../../queries/databaseAccess'
import { useSetDatabasePublicAccess } from '../../queries/databases'

function ExposureBody({
  databaseName,
  network,
}: {
  databaseName: string
  network: DatabaseNetwork
}) {
  const { t } = useTranslation('dbNav')
  const qc = useQueryClient()
  const makePrivate = useMakePrivate(databaseName)
  const setPublic = useSetDatabasePublicAccess()
  const current = currentLevel(network)
  const [selected, setSelected] = useState<ExposureLevel>(current)
  const [bind, setBind] = useState<BindChoice>(currentBind(network))
  const [confirming, setConfirming] = useState(false)
  const refusal = publishDisabledReason(network)
  const published = network.published
  const port = published?.host_port ?? network.internal.port
  const publicHost = bind === 'private' ? '127.0.0.1' : window.location.hostname
  const unchanged =
    current === 'public' && bind === currentBind(network) && !!published

  function done(title: string) {
    toast.add({ title, type: 'success' })
    setConfirming(false)
    void qc.invalidateQueries({
      queryKey: databaseAccessKeys.all(databaseName),
    })
  }
  function fail(err: Error) {
    toast.add({
      title: t('networking.failedToast'),
      description: err.message,
      type: 'error',
    })
  }
  function apply() {
    if (selected === 'private') {
      makePrivate.mutate(undefined, {
        onSuccess: () => done(t('networking.privateToast')),
        onError: fail,
      })
      return
    }
    setPublic.mutate(
      { name: databaseName, bindAddress: bind },
      { onSuccess: () => done(t('networking.publishedToast')), onError: fail },
    )
  }

  const endpoint =
    selected === 'private'
      ? `${network.internal.host}:${network.internal.port}`
      : selected === 'tunnel'
        ? `127.0.0.1:${port}`
        : `${publicHost}:${published?.host_port ?? t('networking.autoPort')}`

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2 space-y-0">
        <CardTitle className="flex items-center gap-1.5 text-sm">
          <GlobeGlassIcon className="size-5" aria-hidden="true" />
          {t('networking.exposureTitle')}
        </CardTitle>
        <div className="flex items-center gap-2">
          <SegmentedControl<ExposureLevel>
            aria-label={t('networking.levelLabel')}
            value={selected}
            onValueChange={setSelected}
            options={LEVELS.map((l) => ({
              value: l,
              label: t(`networking.${l}`),
            }))}
          />
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
          <span>{t(`networking.${selected}Hint`)}</span>
          {current === selected ? (
            <Badge variant="muted">{t('networking.current')}</Badge>
          ) : null}
        </div>
        <dl className="flex flex-wrap items-baseline gap-x-2 text-sm">
          <dt className="text-xs text-muted-foreground">
            {t('networking.endpoint')}
          </dt>
          <dd className="font-mono text-xs">{endpoint}</dd>
        </dl>
        {selected === 'tunnel' ? (
          <div className="space-y-1">
            <p className="text-xs text-muted-foreground">
              {t('networking.tunnelRun')}
            </p>
            <CodeBlock
              code={tunnelCommand(databaseName, port)}
              language="bash"
            />
          </div>
        ) : null}
        {selected === 'public' ? (
          <div className="flex flex-wrap items-end gap-2">
            <div className="space-y-1">
              <Label htmlFor="exposure-bind" className="text-xs">
                {t('networking.bindLabel')}
              </Label>
              <NativeSelect
                id="exposure-bind"
                value={bind}
                disabled={refusal !== null}
                onChange={(e) => setBind(e.target.value as BindChoice)}
              >
                <option value="private">{t('networking.bindLoopback')}</option>
                <option value="public">{t('networking.bindAll')}</option>
              </NativeSelect>
            </div>
          </div>
        ) : null}
        {selected === 'public' && bind === 'public' && refusal === null ? (
          <Alert variant="destructive">
            <AlertDescription>
              {t('networking.bindAllWarning')}
            </AlertDescription>
          </Alert>
        ) : null}
        {selected === 'public' && refusal !== null ? (
          <Alert>
            <AlertDescription>
              {t('networking.publicDisabled', { reason: refusal })}
            </AlertDescription>
          </Alert>
        ) : null}
        {selected !== 'tunnel' ? (
          <Button
            size="sm"
            writeAction
            variant={selected === 'public' ? 'destructive' : 'default'}
            disabled={
              (selected === 'private' ? current === 'private' : unchanged) ||
              (selected === 'public' && refusal !== null) ||
              makePrivate.isPending ||
              setPublic.isPending
            }
            onClick={() => setConfirming(true)}
          >
            {selected === 'private'
              ? t('networking.makePrivate')
              : t('networking.publish')}
          </Button>
        ) : null}
      </CardContent>
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t(
          selected === 'private'
            ? 'networking.makePrivateTitle'
            : 'networking.publishTitle',
          { name: databaseName },
        )}
        description={t(
          selected === 'private'
            ? 'networking.makePrivateBody'
            : bind === 'public'
              ? 'networking.publishBodyAll'
              : 'networking.publishBodyLoopback',
        )}
        confirmLabel={t(
          selected === 'private'
            ? 'networking.makePrivate'
            : 'networking.publish',
        )}
        tone={selected === 'public' ? 'destructive' : 'default'}
        requireTyped={
          selected === 'public' && needsTypedConfirm(current, bind)
            ? databaseName
            : undefined
        }
        pending={makePrivate.isPending || setPublic.isPending}
        onConfirm={apply}
      />
    </Card>
  )
}

/** ExposureCard is the one place that states and changes how reachable the database is: private, tunnel or a published port. */
export function ExposureCard({
  databaseName,
  network,
}: {
  databaseName: string
  network: DatabaseNetwork
}) {
  return (
    <ExposureBody
      key={`${currentLevel(network)}-${currentBind(network)}`}
      databaseName={databaseName}
      network={network}
    />
  )
}
