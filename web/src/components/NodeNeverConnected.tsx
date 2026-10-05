import { useTranslation } from 'react-i18next'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import type { NodeResource } from '../types/nodeDetail'

function isNeverConnected(node: NodeResource): boolean {
  return node.status_reason === 'enrolled_never_connected'
}

export function NodeNeverConnectedBadge({ node }: { node: NodeResource }) {
  const { t } = useTranslation('nodes')
  if (!isNeverConnected(node)) {
    return null
  }
  return (
    <Badge variant="warning" title={t('neverConnected.description')}>
      {t('neverConnected.badge')}
    </Badge>
  )
}

export function NodeNeverConnectedAlert({ node }: { node: NodeResource }) {
  const { t } = useTranslation('nodes')
  if (!isNeverConnected(node)) {
    return null
  }
  return (
    <Alert>
      <WarningIcon className="size-4" aria-hidden="true" />
      <AlertTitle>{t('neverConnected.title')}</AlertTitle>
      <AlertDescription>{t('neverConnected.description')}</AlertDescription>
    </Alert>
  )
}
