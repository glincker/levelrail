import { useTranslation } from 'react-i18next'
import {
  BanGlassIcon,
  CircleCheckGlassIcon,
  CircleGlassIcon,
} from '@thesvg/ui-react'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '../hooks/useCopyToClipboard'
import {
  useControlPlaneRestorePlan,
  type ControlPlaneRestoreStep,
} from '../queries/controlPlaneDr'

function StepIcon({ state }: { state: ControlPlaneRestoreStep['state'] }) {
  if (state === 'done') {
    return (
      <CircleCheckGlassIcon
        className="size-4.5 text-success"
        aria-hidden="true"
      />
    )
  }
  if (state === 'blocked') {
    return (
      <BanGlassIcon className="size-4.5 text-destructive" aria-hidden="true" />
    )
  }
  return (
    <CircleGlassIcon
      className="size-4.5 text-muted-foreground"
      aria-hidden="true"
    />
  )
}

export function ControlPlaneDrRestore({ configured }: { configured: boolean }) {
  const { t } = useTranslation('settings')
  const plan = useControlPlaneRestorePlan(configured)
  const { copied, copy } = useCopyToClipboard()

  if (!configured || plan.isLoading || plan.isError || !plan.data) {
    return null
  }
  const data = plan.data

  return (
    <section
      className="space-y-3 border-t border-border pt-4"
      aria-label={t('controlPlaneDrRestore.title')}
    >
      <div>
        <h3 className="text-sm font-medium">
          {t('controlPlaneDrRestore.title')}
        </h3>
        <p className="text-xs text-muted-foreground">
          {t('controlPlaneDrRestore.description')}
        </p>
      </div>
      {data.backup === undefined ? (
        <Banner variant="warning" title={t('controlPlaneDrRestore.noBackup')} />
      ) : (
        <p className="text-sm">
          {t('controlPlaneDrRestore.newest', {
            key: data.backup.key,
            when: new Date(data.backup.created_at).toLocaleString(),
          })}
        </p>
      )}
      <ol className="space-y-2 text-sm">
        {data.steps.map((s) => (
          <li key={s.id} className="flex items-start gap-2">
            <StepIcon state={s.state} />
            <div>
              <span className="font-medium">
                {t(`controlPlaneDrRestore.steps.${s.id}.title`, {
                  defaultValue: s.id,
                })}
              </span>
              <p className="text-xs text-muted-foreground">
                {t(`controlPlaneDrRestore.steps.${s.id}.body`, {
                  defaultValue: '',
                })}
              </p>
            </div>
          </li>
        ))}
      </ol>
      {data.warnings.map((w) => (
        <Banner
          key={w}
          variant="warning"
          title={t(`controlPlaneDrRestore.warnings.${w}`, { defaultValue: w })}
        />
      ))}
      {data.command !== undefined ? (
        <details className="text-sm">
          <summary className="cursor-pointer text-muted-foreground">
            {t('controlPlaneDrRestore.viaCli')}
          </summary>
          <pre className="mt-2 overflow-auto rounded-md bg-muted p-3 font-mono text-xs break-all whitespace-pre-wrap">
            {data.command}
          </pre>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="mt-2"
            onClick={() => {
              copy(data.command ?? '')
            }}
          >
            {copied
              ? t('controlPlaneDrRestore.copied')
              : t('controlPlaneDrRestore.copy')}
          </Button>
        </details>
      ) : null}
    </section>
  )
}
