import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { GoLiveChecklist } from '../../../components/golive/GoLiveChecklist'

export const Route = createFileRoute('/apps/$name/go-live')({
  validateSearch: (search: Record<string, unknown>): { domain?: string } =>
    typeof search.domain === 'string' && search.domain
      ? { domain: search.domain }
      : {},
  component: GoLiveSection,
})

function GoLiveSection() {
  const { name } = Route.useParams()
  const { domain } = Route.useSearch()
  const { t } = useTranslation('golive')
  return (
    <div className="mx-auto max-w-3xl space-y-4">
      <p className="text-sm text-muted-foreground">{t('intro')}</p>
      <GoLiveChecklist app={name} domain={domain ?? ''} />
    </div>
  )
}
