import { useTranslation } from 'react-i18next'
import { ArrowCounterClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
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
  useAutoRollbackSLOBurn,
  useSetAutoRollbackSLOBurn,
  type SLOBurnAutoRollbackMode,
} from '../queries/autoRollbackSLOBurn'

// Order only: label/description come from the deploys.autoRollbackSloBurn.modes
// namespace, keyed by this same value, so a translation is never missing.
const MODE_VALUES: SLOBurnAutoRollbackMode[] = [
  'off',
  'auto',
  'dry_run',
  'pause_for_human',
]

// AutoRollbackSLOBurnCard is the opt-in mode selector for
// internal/alerting.MaybeAutoRollbackOnSLOBurn (GET/PUT
// /api/v1/apps/{name}/auto-rollback-slo-burn): off by default, the same
// risky-by-default-feature-is-opt-in shape AutoRollbackCard's own
// crashloop toggle already establishes, but a mode selector rather than a
// switch since there are three distinct reactions to choose between, not
// just on/off. Rendered alongside AutoRollbackCard on the Deploys route.
export function AutoRollbackSLOBurnCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const setting = useAutoRollbackSLOBurn(appName)
  const setMode = useSetAutoRollbackSLOBurn(appName)

  const modeOptions = MODE_VALUES.map((value) => ({
    value,
    label: t(`autoRollbackSloBurn.modes.${value}.label`),
    description: t(`autoRollbackSloBurn.modes.${value}.description`),
  }))

  function change(next: SLOBurnAutoRollbackMode | null) {
    if (!next || next === setting.data.mode) return
    setMode.mutate(next, {
      onSuccess: () => {
        const label = modeOptions.find((o) => o.value === next)?.label ?? next
        toast.add({
          title: t('autoRollbackSloBurn.toast.modeSet', { mode: label }),
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({
          title: t('autoRollbackSloBurn.toast.errorTitle'),
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  const current = modeOptions.find((o) => o.value === setting.data.mode)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ArrowCounterClockwiseIcon className="size-4 text-muted-foreground" />
          {t('autoRollbackSloBurn.title')}
          <InfoTip
            label={t('autoRollbackSloBurn.infoTipLabel')}
            helpPath="/observability#alert-rules"
            helpLabel={t('autoRollbackSloBurn.helpLabel')}
          >
            <ul className="space-y-1.5">
              {modeOptions.map((opt) => (
                <li key={opt.value}>
                  <span className="font-medium text-foreground">
                    {opt.label}:
                  </span>{' '}
                  {opt.description}
                </li>
              ))}
            </ul>
          </InfoTip>
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-sm font-medium text-foreground">
              {t('autoRollbackSloBurn.modeLabel')}
            </p>
            <p className="text-sm text-muted-foreground">
              {current?.description ??
                t('autoRollbackSloBurn.defaultDescription')}
            </p>
          </div>
          <Select
            value={setting.data.mode}
            onValueChange={change}
            disabled={setMode.isPending}
          >
            <SelectTrigger
              className="w-44"
              aria-label={t('autoRollbackSloBurn.modeAriaLabel')}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {modeOptions.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </CardContent>
    </Card>
  )
}
