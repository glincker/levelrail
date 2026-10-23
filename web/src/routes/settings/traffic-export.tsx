import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ShareNetworkIcon } from '@phosphor-icons/react/dist/ssr'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { TrafficExportCard } from '../../components/trafficExport/TrafficExportCard'

export const Route = createFileRoute('/settings/traffic-export')({
  component: TrafficExportPage,
})

function TrafficExportPage() {
  const { t } = useTranslation('trafficExport')
  return (
    <PageTemplate
      icon={<ShareNetworkIcon />}
      title={t('title')}
      description={t('description')}
    >
      <TrafficExportCard />
    </PageTemplate>
  )
}
