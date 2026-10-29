import { useTranslation } from 'react-i18next'
import { ActivityGlassIcon, CircleCheckGlassIcon } from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { FixAction } from '../fixit/FixAction'
import { useSettingsFixes } from '../../queries/fixit'

/** Every misconfiguration the control plane can detect itself, each with its typed Fix or Re-check. */
export function SettingsHealthCard() {
  const { t } = useTranslation('fixit')
  const { data, isPending } = useSettingsFixes()
  const fixes = data ?? []
  const failing = fixes.filter((f) => f.failing)
  const healthy = fixes.filter((f) => !f.failing)

  return (
    <Card id="settings-health">
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <ActivityGlassIcon className="size-5" aria-hidden="true" />
          </div>
          <div>
            <CardTitle>{t('settingsHealth.title')}</CardTitle>
            <CardDescription>
              {isPending
                ? t('settingsHealth.loading')
                : t('settingsHealth.summary', {
                    failing: failing.length,
                    total: fixes.length,
                  })}
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {failing.map((f) => (
          <FixAction key={f.id} fix={f} />
        ))}
        {!isPending && failing.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('settingsHealth.allGood')}
          </p>
        ) : null}
        {healthy.length > 0 ? (
          <ul className="space-y-1">
            {healthy.map((f) => (
              <li
                key={f.id}
                className="flex items-start gap-2 text-sm text-muted-foreground"
              >
                <CircleCheckGlassIcon
                  className="mt-0.5 size-5 shrink-0 text-green-700 dark:text-green-400"
                  aria-hidden="true"
                />
                <span>{f.detail || f.title}</span>
              </li>
            ))}
          </ul>
        ) : null}
      </CardContent>
    </Card>
  )
}
