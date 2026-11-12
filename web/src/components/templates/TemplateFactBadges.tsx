import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  BadgeCheckGlassIcon,
  FlaskConicalGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import type { TemplateFacts } from '../../queries/serviceTemplates'

/** isDeprecated is true for deprecated and yanked entries, which stay listed. */
function isDeprecated(facts: TemplateFacts): boolean {
  return facts.status === 'deprecated' || facts.status === 'yanked'
}

/**
 * TemplateFactBadges shows the marketplace facts of a template: license class,
 * what was verified, and a deprecation notice. Unknown facts render nothing.
 * `full` adds the verification sentence and the deprecation reason.
 */
export function TemplateFactBadges({
  facts,
  full = false,
}: {
  facts: TemplateFacts
  full?: boolean
}) {
  const { t } = useTranslation('templates')
  const license = facts.license
  const verification = facts.verification
  const deprecated = isDeprecated(facts)
  const preview = facts.preview === true
  if (!license && !verification && !deprecated && !preview) return null

  return (
    <div className="flex flex-wrap items-center gap-2">
      {license && (license.class || license.spdx) ? (
        <Badge
          variant="outline"
          className="text-xs"
          title={t('facts.license.title', { spdx: license.spdx })}
        >
          {license.class
            ? t(`facts.license.classes.${license.class}`)
            : license.spdx}
        </Badge>
      ) : null}
      {full && verification && verification.arches.length > 0 ? (
        <Badge variant="success" className="gap-1 text-xs">
          <BadgeCheckGlassIcon className="size-4.5" aria-hidden="true" />
          {t('facts.verifiedOn', {
            arches: verification.arches.join(', '),
            tag: verification.tag,
          })}
        </Badge>
      ) : null}
      {preview ? (
        <Badge
          variant="muted"
          className="gap-1 text-xs"
          title={t('facts.preview.hint')}
        >
          <FlaskConicalGlassIcon className="size-4.5" aria-hidden="true" />
          {t('facts.preview.badge')}
        </Badge>
      ) : null}
      {deprecated ? (
        <Badge variant="warning" className="gap-1 text-xs">
          <TriangleAlertGlassIcon className="size-4.5" aria-hidden="true" />
          {t(facts.status === 'yanked' ? 'facts.yanked' : 'facts.deprecated')}
        </Badge>
      ) : null}
      {full && deprecated ? (
        <p className="text-sm text-foreground/70">
          {facts.status_reason}{' '}
          {facts.replaced_by ? (
            <Link
              to="/templates/$id"
              params={{ id: facts.replaced_by }}
              className="text-primary underline underline-offset-4"
            >
              {t('facts.useInstead', { id: facts.replaced_by })}
            </Link>
          ) : null}
        </p>
      ) : null}
    </div>
  )
}
