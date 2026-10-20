import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { fillTemplateTarget, isTemplateKey } from '@/lib/dashboardGrid'
import { useAppListOptional } from '@/queries/apps'
import { useDashboardTemplates } from '@/queries/customDashboards'
import type { DashboardDoc, DashboardTemplate } from '@/types/dashboards'

function TemplateCard({
  tpl,
  apps,
  busy,
  onPick,
}: {
  tpl: DashboardTemplate
  apps: string[]
  busy: boolean
  onPick: (doc: DashboardDoc) => void
}) {
  const { t } = useTranslation('dashboards')
  const [app, setApp] = useState('')
  const needsApp = tpl.needs_target === 'app'
  const key = tpl.key
  const name = isTemplateKey(key) ? t(`template.${key}.name`) : tpl.doc.name
  const description = isTemplateKey(key)
    ? t(`template.${key}.description`)
    : (tpl.doc.description ?? '')
  return (
    <li className="flex flex-col gap-3 rounded-lg border border-border bg-card p-4">
      <div>
        <h3 className="text-sm font-medium">{name}</h3>
        <p className="text-sm text-muted-foreground">{description}</p>
      </div>
      {needsApp ? (
        <Select
          value={app}
          onValueChange={(v) => {
            if (v) setApp(v)
          }}
        >
          <SelectTrigger
            className="w-full"
            aria-label={t('template.pickApp', { name })}
          >
            <SelectValue placeholder={t('add.targetPlaceholder')} />
          </SelectTrigger>
          <SelectContent>
            {apps.map((a) => (
              <SelectItem key={a} value={a}>
                {a}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : null}
      <Button
        className="mt-auto"
        disabled={busy || (needsApp && app === '')}
        onClick={() =>
          onPick({
            ...tpl.doc,
            widgets: needsApp
              ? fillTemplateTarget(tpl.doc.widgets, app)
              : tpl.doc.widgets,
          })
        }
      >
        {t('template.use')}
      </Button>
    </li>
  )
}

/** One click starter dashboards; app based ones ask which app to read from first. */
export function TemplatePicker({
  busy,
  compact = false,
  onPick,
}: {
  busy: boolean
  compact?: boolean
  onPick: (doc: DashboardDoc) => void
}) {
  const { t } = useTranslation('dashboards')
  const templates = useDashboardTemplates()
  const apps = useAppListOptional()
  const names = (apps.data ?? []).map((a) => a.name)
  return (
    <section aria-label={t('template.label')}>
      <ul className={compact ? 'grid gap-4' : 'grid gap-4 md:grid-cols-3'}>
        {(templates.data ?? []).map((tpl) => (
          <TemplateCard
            key={tpl.key}
            tpl={tpl}
            apps={names}
            busy={busy}
            onPick={onPick}
          />
        ))}
      </ul>
    </section>
  )
}
