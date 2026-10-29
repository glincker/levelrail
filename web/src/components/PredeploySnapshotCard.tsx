import { useTranslation } from 'react-i18next'
import { CameraGlassIcon } from '@thesvg/ui-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { InfoTip } from '@/components/kit'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import {
  usePredeploySettings,
  useSetPredeploySettings,
} from '../queries/predeploySnapshot'
import { useBackupTargetsOptional } from '../queries/backupTargets'
import { PREDEPLOY_MODES, type PredeployMode } from '../types/predeploySnapshot'

// Sentinel for "no explicit target": the API treats an empty target_id as the only configured one.
const AUTO_TARGET = '__auto__'

function isMode(v: string | null): v is PredeployMode {
  return PREDEPLOY_MODES.some((m) => m === v)
}

export function PredeploySnapshotCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const setting = usePredeploySettings(appName)
  const targets = useBackupTargetsOptional()
  const save = useSetPredeploySettings(appName)

  const mode = setting.data?.mode ?? 'off'
  const targetId = setting.data?.target_id ?? ''

  function update(next: { mode: PredeployMode; target_id: string }) {
    save.mutate(next, {
      onSuccess: () => {
        toast.add({
          title: t('predeploySnapshot.toast.saved'),
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({
          title: t('predeploySnapshot.toast.errorTitle'),
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CameraGlassIcon className="size-4.5 text-muted-foreground" />
          {t('predeploySnapshot.title')}
          <InfoTip
            label={t('predeploySnapshot.infoTipLabel')}
            helpPath="/predeploy-snapshot"
            helpLabel={t('predeploySnapshot.helpLabel')}
          >
            <ul className="space-y-1.5">
              {PREDEPLOY_MODES.map((m) => (
                <li key={m}>
                  <span className="font-medium text-foreground">
                    {t(`predeploySnapshot.modes.${m}`)}:
                  </span>{' '}
                  {t(`predeploySnapshot.modeHints.${m}`)}
                </li>
              ))}
            </ul>
          </InfoTip>
        </CardTitle>
      </CardHeader>
      <CardContent>
        {setting.isError ? (
          <p role="alert" className="text-sm text-muted-foreground">
            {t('predeploySnapshot.unavailable')}
          </p>
        ) : (
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
            <p className="min-w-0 flex-1 text-sm text-muted-foreground">
              {t(`predeploySnapshot.modeHints.${mode}`)}
            </p>
            <Select
              value={mode}
              onValueChange={(v) => {
                if (isMode(v)) update({ mode: v, target_id: targetId })
              }}
              disabled={setting.isPending || save.isPending}
            >
              <SelectTrigger
                className="w-40"
                aria-label={t('predeploySnapshot.modeAriaLabel')}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PREDEPLOY_MODES.map((m) => (
                  <SelectItem key={m} value={m}>
                    {t(`predeploySnapshot.modes.${m}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {mode !== 'off' ? (
              <Select
                value={targetId === '' ? AUTO_TARGET : targetId}
                onValueChange={(v) =>
                  update({
                    mode,
                    target_id: v === AUTO_TARGET ? '' : (v ?? ''),
                  })
                }
                disabled={save.isPending}
              >
                <SelectTrigger
                  className="w-48"
                  aria-label={t('predeploySnapshot.targetAriaLabel')}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={AUTO_TARGET}>
                    {t('predeploySnapshot.targetAuto')}
                  </SelectItem>
                  {(targets.data ?? []).map((target) => (
                    <SelectItem key={target.id} value={target.id}>
                      {target.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : null}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
