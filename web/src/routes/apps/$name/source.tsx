import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { GitSourceConnectDialog } from '../../../components/GitSourceConnectDialog'
import { useApp } from '../../../queries/apps'
import { GitSourceCard } from '../../../components/GitSourceCard'
import { InlineDockerfileSourceSlot } from '../../../components/InlineDockerfileSourceSlot'
import { GitPushRemoteCard } from '../../../components/GitPushRemoteCard'
import { GitDeploySettingsCard } from '../../../components/GitDeploySettingsCard'
import { CiGateSettings } from '../../../components/apps/CiGateSettings'
import { GitTriggerPatternsSlot } from '../../../components/GitTriggerPatternsSlot'
import { WebhookDeliveriesPanel } from '../../../components/WebhookDeliveriesPanel'

// Former Overview-page cards, split out here since a preview environment
// is meaningless without a connected git source: the two belong together
// as one "where deploys come from" concern, distinct from Deploy
// settings (port/strategy) and Integrations (storage/log drain/database).
export const Route = createFileRoute('/apps/$name/source')({
  component: SourceSection,
  validateSearch: (search: Record<string, unknown>): { connect?: boolean } =>
    search.connect === true || search.connect === '1' || search.connect === 1
      ? { connect: true }
      : {},
})

function SourceSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)
  const { connect } = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const { t } = useTranslation('gitSource')
  const setConnect = (open: boolean) =>
    void navigate({ search: open ? { connect: true } : {}, replace: true })

  return (
    <div className="space-y-6">
      <div className="flex justify-end">
        <Button type="button" size="sm" onClick={() => setConnect(true)}>
          {t('dialog.open')}
        </Button>
      </div>
      <GitSourceConnectDialog
        appName={name}
        open={connect === true}
        onOpenChange={setConnect}
      />
      <GitSourceCard app={app} />
      <InlineDockerfileSourceSlot appName={name} />
      <GitPushRemoteCard appName={name} />
      <GitDeploySettingsCard appName={name} />
      <GitTriggerPatternsSlot appName={name} />
      <CiGateSettings appName={name} />
      <WebhookDeliveriesPanel app={app} />
    </div>
  )
}
