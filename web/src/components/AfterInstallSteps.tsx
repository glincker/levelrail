import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ExternalLinkGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { useServiceTemplate } from '../queries/serviceTemplates'
import type { AppDetail } from '../types/appDetail'

type StepsApp = Pick<AppDetail, 'name' | 'domains' | 'template_id'>

function dismissKey(name: string): string {
  return `lr.afterInstall.dismissed.${name}`
}

function readDismissed(name: string): boolean {
  try {
    return localStorage.getItem(dismissKey(name)) === '1'
  } catch {
    return false
  }
}

// Per-template "what to do next" list (internal/catalog/next_steps.json).
// Renders nothing for apps from templates without recorded steps.
export function AfterInstallSteps({ app }: { app: StepsApp }) {
  const { t } = useTranslation('nextSteps')
  const [dismissed, setDismissed] = useState(() => readDismissed(app.name))
  const template = useServiceTemplate(app.template_id ?? '')
  const steps = template.data?.next_steps ?? []
  if (dismissed || steps.length === 0) return null

  const host = app.domains?.[0]
  const dismiss = () => {
    try {
      localStorage.setItem(dismissKey(app.name), '1')
    } catch {
      // Storage can be blocked; the banner just returns next load.
    }
    setDismissed(true)
  }

  return (
    <div className="rounded-lg border border-border bg-card p-3">
      <div className="flex items-start justify-between gap-2">
        <p className="text-sm font-medium">{t('title', { name: app.name })}</p>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          aria-label={t('dismiss')}
          onClick={dismiss}
        >
          <XGlassIcon className="size-5" aria-hidden="true" />
        </Button>
      </div>
      <ol className="mt-2 list-decimal space-y-1 pl-5 text-sm text-foreground/80">
        {steps.map((step) => (
          <li key={step.key}>
            {step.key === 'open_admin' && host && step.path ? (
              <a
                href={`https://${host}${step.path}`}
                target="_blank"
                rel="noreferrer noopener"
                className="inline-flex items-center gap-1 text-primary underline underline-offset-4"
              >
                {t(`steps.${step.key}`)}
                <ExternalLinkGlassIcon className="size-4" aria-hidden="true" />
              </a>
            ) : step.key === 'point_domain' ? (
              <Link
                to="/apps/$name/domains"
                params={{ name: app.name }}
                className="text-primary underline underline-offset-4"
              >
                {t(`steps.${step.key}`)}
              </Link>
            ) : (
              t(`steps.${step.key}`)
            )}
          </li>
        ))}
      </ol>
    </div>
  )
}
