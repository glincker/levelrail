import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  MinusCircleIcon,
  ShareNetworkIcon,
  WarningCircleIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import {
  useMeshSettings,
  useUpdateMeshSettings,
  type MeshCheck,
  type MeshCheckStatus,
  type MeshSettings,
} from '../../queries/meshSettings'
import { meshCardState } from '../../lib/meshCardState'
import { RestartControlPlaneButton } from '../RestartControlPlaneButton'

const CHECK_ICON: Record<MeshCheckStatus, typeof CheckCircleIcon> = {
  pass: CheckCircleIcon,
  warn: WarningCircleIcon,
  fail: XCircleIcon,
}

const CHECK_TONE: Record<MeshCheckStatus, string> = {
  pass: 'text-emerald-600 dark:text-emerald-400',
  warn: 'text-amber-600 dark:text-amber-400',
  fail: 'text-destructive',
}

function CheckRow({ check }: { check: MeshCheck }) {
  const { t } = useTranslation('network')
  const Icon = CHECK_ICON[check.status] ?? MinusCircleIcon
  return (
    <li className="flex items-start gap-2 text-sm">
      <Icon
        className={`mt-0.5 size-4 shrink-0 ${CHECK_TONE[check.status]}`}
        aria-hidden="true"
      />
      <div className="min-w-0">
        <p className="font-medium text-foreground">
          {t(`mesh.check.${check.id}`, { defaultValue: check.id })}
          <span className="sr-only"> {t(`mesh.status.${check.status}`)}</span>
        </p>
        <p className="text-muted-foreground">{check.detail}</p>
        {check.hint && check.status !== 'pass' ? (
          <p className="font-mono text-xs break-words text-foreground">
            {t('mesh.fix', { hint: check.hint })}
          </p>
        ) : null}
      </div>
    </li>
  )
}

export function MeshSettingsCard() {
  const { data: settings } = useMeshSettings()
  if (!settings) return null
  return <MeshSettingsBody settings={settings} />
}

export function MeshSettingsBody({ settings }: { settings: MeshSettings }) {
  const { t } = useTranslation('network')
  const update = useUpdateMeshSettings()
  const [confirm, setConfirm] = useState<'enable' | 'disable' | null>(null)
  const state = meshCardState(settings)
  const pinned = settings.source === 'env'
  const canToggle = !pinned
  const showEnable = canToggle && !settings.configured
  const showDisable = canToggle && settings.configured

  function submit(enabled: boolean) {
    update.mutate(
      { enabled, force: !enabled && settings.remote_nodes > 0 },
      {
        onSuccess: () => {
          setConfirm(null)
          toast.add({ title: t('mesh.saved'), type: 'success' })
        },
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          <ShareNetworkIcon className="size-4" aria-hidden="true" />
          {t('mesh.title')}
          <Badge variant={state === 'on' ? 'success' : 'outline'}>
            {t(`mesh.state.${state}`)}
          </Badge>
        </CardTitle>
        <CardDescription>{t('mesh.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {pinned ? (
          <Alert>
            <AlertDescription>
              {t('mesh.pinned', { envVar: settings.env_var })}
            </AlertDescription>
          </Alert>
        ) : null}
        {settings.start_error && state === 'error' ? (
          <Alert variant="destructive">
            <AlertDescription>
              {t('mesh.startError', { error: settings.start_error })}
            </AlertDescription>
          </Alert>
        ) : null}
        {settings.restart_required ? (
          <Alert>
            <AlertDescription className="space-y-1">
              <p>{t('mesh.restartRequired')}</p>
              {settings.restart_hint ? (
                <p className="font-mono text-xs">
                  {t('mesh.restartCommand')} {settings.restart_hint}
                </p>
              ) : null}
              <RestartControlPlaneButton />
            </AlertDescription>
          </Alert>
        ) : null}

        {!settings.effective ? (
          <div className="space-y-2">
            <h3 className="text-sm font-medium text-foreground">
              {t('mesh.readiness')}
            </h3>
            <ul className="space-y-2">
              {settings.checks.map((c) => (
                <CheckRow key={c.id} check={c} />
              ))}
            </ul>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            {t('mesh.otherNodes', { count: settings.remote_nodes })}
          </p>
        )}

        <div className="flex flex-wrap items-center gap-2">
          {showEnable ? (
            <Button
              onClick={() => setConfirm('enable')}
              disabled={!settings.can_enable}
            >
              {t('mesh.enable')}
            </Button>
          ) : null}
          {showDisable ? (
            <Button variant="outline" onClick={() => setConfirm('disable')}>
              {t('mesh.disable')}
            </Button>
          ) : null}
          {showEnable && !settings.can_enable ? (
            <p className="text-sm text-muted-foreground">{t('mesh.blocked')}</p>
          ) : null}
        </div>
      </CardContent>

      <ConfirmDialog
        open={confirm === 'enable'}
        onOpenChange={(o) => setConfirm(o ? 'enable' : null)}
        title={t('mesh.enableTitle')}
        description={t('mesh.enableDescription')}
        consequences={[
          t('mesh.enableConsequence1'),
          t('mesh.enableConsequence2'),
          t('mesh.enableConsequence3'),
        ]}
        confirmLabel={t('mesh.enable')}
        pending={update.isPending}
        error={update.error?.message ?? null}
        onConfirm={() => submit(true)}
      />
      <ConfirmDialog
        open={confirm === 'disable'}
        onOpenChange={(o) => setConfirm(o ? 'disable' : null)}
        title={t('mesh.disableTitle')}
        description={t('mesh.disableDescription')}
        consequences={[
          ...(settings.remote_nodes > 0
            ? [t('mesh.disableNodes', { count: settings.remote_nodes })]
            : []),
          t('mesh.disableConsequence1'),
          t('mesh.disableConsequence2'),
        ]}
        confirmLabel={t('mesh.disable')}
        tone="destructive"
        pending={update.isPending}
        error={update.error?.message ?? null}
        onConfirm={() => submit(false)}
      />
    </Card>
  )
}
