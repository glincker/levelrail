import { useTranslation } from 'react-i18next'
import { KeyGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import type { StackTemplateSummary } from '../../../types/stackTemplates'

export function StackTemplateCard({
  template,
  onOpen,
}: Readonly<{
  template: StackTemplateSummary
  onOpen: () => void
}>) {
  const { t } = useTranslation('stacks')
  return (
    <button
      type="button"
      onClick={onOpen}
      aria-label={t('templates.open', { name: template.title })}
      className="flex w-full flex-col gap-2 rounded-xl border bg-card p-3 text-left transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none motion-reduce:transition-none"
    >
      <span className="font-mono text-sm font-medium">{template.title}</span>
      <span className="text-xs text-muted-foreground">
        {template.description}
      </span>
      <span className="mt-auto flex flex-wrap items-center gap-1 pt-1">
        {template.resource_kinds.map((kind) => (
          <Badge key={kind} variant="muted">
            {kind}
          </Badge>
        ))}
        {template.secret_names.length > 0 ? (
          <Badge variant="warning">
            <KeyGlassIcon size="1.4em" aria-hidden="true" />
            {t('templates.secretCount', {
              count: template.secret_names.length,
            })}
          </Badge>
        ) : null}
      </span>
    </button>
  )
}
