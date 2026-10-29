import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Card, CardContent } from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { SettingsCardSkeleton } from '@/components/settings/SettingsSkeletons'
import { useNodeProviders } from '../queries/nodeProvision'
import { NodeProviderRow } from './nodeProviders/NodeProviderRow'

// Instance-level cloud provider credentials for "nodes provision" and the
// Add node wizard: GET/POST /api/v1/node-providers. One row per provider,
// with its credential fields in a sheet, the same shape as the OAuth page.
export function NodeProviderCredentialsCard() {
  const { t } = useTranslation('settings')
  const providers = useNodeProviders()

  if (providers.isLoading) {
    return <SettingsCardSkeleton description={false} rows={3} />
  }

  if (providers.isError) {
    return (
      <Alert variant="destructive">
        <TriangleAlertGlassIcon className="text-tone-warning" size="1.2em" />
        <AlertDescription>{providers.error.message}</AlertDescription>
      </Alert>
    )
  }

  const data = providers.data ?? []
  if (data.length === 0) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-sm text-muted-foreground">
          {t('nodeProviders.empty')}
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardContent>
        <ul className="divide-y divide-border">
          {data.map((p) => (
            <NodeProviderRow key={p.provider} provider={p} />
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
