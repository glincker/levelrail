import { useTranslation } from 'react-i18next'
import { LoaderGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import type { ProxyInstallPlan } from '../../queries/proxyInstall'

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5 sm:flex-row sm:gap-3">
      <dt className="w-28 shrink-0 text-xs text-muted-foreground">{label}</dt>
      <dd className="min-w-0 font-mono text-xs break-all text-foreground">
        {value}
      </dd>
    </div>
  )
}

export function ProxyInstallPlanView({
  plan,
  applying,
  onConfirm,
  onCancel,
}: {
  plan: ProxyInstallPlan
  applying: boolean
  onConfirm: () => void
  onCancel: () => void
}) {
  const { t } = useTranslation('domains')
  return (
    <div className="min-w-0 space-y-3 rounded-lg border border-border bg-muted/30 p-4">
      <h4 className="text-sm font-medium text-foreground">
        {t('proxyMissing.plan.title')}
      </h4>
      <dl className="space-y-1">
        <Row label={t('proxyMissing.plan.image')} value={plan.image} />
        <Row label={t('proxyMissing.plan.container')} value={plan.container} />
        <Row
          label={t('proxyMissing.plan.ports')}
          value={plan.ports.join(', ')}
        />
        <Row label={t('proxyMissing.plan.restart')} value={plan.restart} />
        {plan.mounts.map((m) => (
          <Row
            key={m.container}
            label={t('proxyMissing.plan.mount')}
            value={`${m.host} -> ${m.container}`}
          />
        ))}
      </dl>
      {plan.files.map((f) => (
        <div key={f.path} className="min-w-0 space-y-1">
          <p className="font-mono text-xs break-all text-muted-foreground">
            {t('proxyMissing.plan.file', { path: f.path })}
          </p>
          <pre className="max-h-60 overflow-auto rounded-md border border-border bg-background p-3 font-mono text-xs">
            {f.content}
          </pre>
        </div>
      ))}
      <p className="text-xs text-muted-foreground">
        {t('proxyMissing.plan.safety')}
      </p>
      <div className="flex flex-wrap items-center gap-2">
        <Button type="button" disabled={applying} onClick={onConfirm}>
          {applying ? (
            <LoaderGlassIcon
              size="1.4em"
              className="animate-spin"
              aria-hidden="true"
            />
          ) : null}
          {applying
            ? t('proxyMissing.plan.installing')
            : t('proxyMissing.plan.confirm')}
        </Button>
        <Button
          type="button"
          variant="ghost"
          disabled={applying}
          onClick={onCancel}
        >
          {t('proxyMissing.plan.cancel')}
        </Button>
      </div>
    </div>
  )
}
