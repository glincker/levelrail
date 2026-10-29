import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  ShieldCheckGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import type {
  RequirementsLevel,
  TemplateFinding,
  TemplateRequirements,
} from '../../queries/serviceTemplates'

const LEVEL_VARIANT = {
  standard: 'success',
  review: 'warning',
  elevated: 'destructive',
} as const satisfies Record<RequirementsLevel, string>

const SEVERITY_VARIANT = {
  info: 'muted',
  review: 'warning',
  elevated: 'destructive',
} as const satisfies Record<TemplateFinding['severity'], string>

export function TemplateRequirementsCard({
  requirements,
  acknowledged,
  onAcknowledgedChange,
}: {
  requirements: TemplateRequirements
  acknowledged?: boolean
  onAcknowledgedChange?: (value: boolean) => void
}) {
  const { t } = useTranslation('templates')
  const attention = requirements.findings.filter((f) => f.severity !== 'info')
  const elevated = requirements.level === 'elevated'

  function findingText(f: TemplateFinding): string {
    return t(`requirements.finding.${f.code}`, { defaultValue: f.message })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2 text-sm">
          <ShieldCheckGlassIcon
            className="size-4.5 text-tone-success"
            aria-hidden="true"
          />
          {t('requirements.title')}
          <Badge variant={LEVEL_VARIANT[requirements.level]}>
            {t(`requirements.level.${requirements.level}`)}
          </Badge>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {elevated ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.2em"
            />
            <AlertDescription>
              {t('requirements.elevatedWarning')}
            </AlertDescription>
          </Alert>
        ) : null}

        {attention.length > 0 ? (
          <ul className="space-y-1.5 text-sm">
            {attention.map((f) => (
              <li key={f.code} className="flex items-start gap-2">
                <Badge variant={SEVERITY_VARIANT[f.severity]}>
                  {t(`requirements.level.${f.severity}`, {
                    defaultValue: f.severity,
                  })}
                </Badge>
                <span>{findingText(f)}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">
            {t('requirements.needsNone')}
          </p>
        )}

        <Collapsible>
          <CollapsibleTrigger className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">
            {t('requirements.showAll')}
            <ChevronDownGlassIcon className="size-3.5" aria-hidden="true" />
          </CollapsibleTrigger>
          <CollapsiblePanel>
            <div className="mt-2 space-y-3">
              <section className="space-y-1">
                <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                  {t('requirements.needsTitle')}
                </h3>
                <ul className="space-y-1 text-sm">
                  {requirements.findings.map((f) => (
                    <li key={f.code}>
                      {findingText(f)}
                      {f.services?.length ? (
                        <span className="text-xs text-muted-foreground">
                          {' '}
                          (
                          {t('requirements.services', {
                            services: f.services.join(', '),
                          })}
                          {f.detail ? `, ${f.detail}` : ''})
                        </span>
                      ) : null}
                    </li>
                  ))}
                </ul>
              </section>
              <section className="space-y-1">
                <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                  {t('requirements.addsTitle')}
                </h3>
                <ul className="list-disc space-y-1 pl-4 text-sm">
                  {requirements.protections.map((p) => (
                    <li key={p.code}>
                      {t(`requirements.protection.${p.code}`, {
                        defaultValue: p.label,
                      })}
                    </li>
                  ))}
                </ul>
              </section>
            </div>
          </CollapsiblePanel>
        </Collapsible>

        {elevated && onAcknowledgedChange ? (
          <label className="flex items-start gap-2 text-sm">
            <Checkbox
              checked={acknowledged === true}
              onCheckedChange={(v) => onAcknowledgedChange(v === true)}
            />
            <span>{t('requirements.acknowledge')}</span>
          </label>
        ) : null}
      </CardContent>
    </Card>
  )
}
