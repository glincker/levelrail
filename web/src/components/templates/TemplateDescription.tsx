import { useTranslation } from 'react-i18next'
import { InfoIcon } from '@phosphor-icons/react/dist/ssr'

interface TemplateDescriptionProps {
  description?: string
  slogan?: string
}

/** TemplateDescription shows operator setup notes as plain text, one line per row. */
export function TemplateDescription({
  description,
  slogan,
}: TemplateDescriptionProps) {
  const { t } = useTranslation('templates')
  const text = description?.trim()
  if (!text || text === slogan?.trim()) return null
  return (
    <div className="min-w-0 rounded-md border border-border bg-muted/40 p-2 text-xs break-words [overflow-wrap:anywhere] text-muted-foreground">
      <p className="mb-1 flex items-center gap-1 font-medium text-foreground">
        <InfoIcon className="size-3" aria-hidden="true" />
        {t('description.label')}
      </p>
      <p className="min-w-0 whitespace-pre-line">{text}</p>
    </div>
  )
}
