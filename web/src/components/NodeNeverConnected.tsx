import { useTranslation } from 'react-i18next'
import { useNavigate } from '@tanstack/react-router'
import { BroomGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useDeleteNode } from '../queries/nodes'
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
  const navigate = useNavigate()
  const deleteNode = useDeleteNode()
  if (!isNeverConnected(node)) {
    return null
  }
  return (
    <Alert>
      <TriangleAlertGlassIcon
        className="size-6 text-tone-warning"
        aria-hidden="true"
      />
      <AlertTitle>{t('neverConnected.title')}</AlertTitle>
      <AlertDescription>{t('neverConnected.description')}</AlertDescription>
      <div className="mt-3 flex items-center gap-3">
        <Button
          type="button"
          size="sm"
          disabled={deleteNode.isPending}
          onClick={() => {
            deleteNode.mutate(node.id, {
              onSuccess: () => {
                toast.add({
                  title: t('neverConnected.cleanedUp', { name: node.name }),
                  type: 'success',
                })
                void navigate({ to: '/nodes' })
              },
            })
          }}
        >
          <BroomGlassIcon className="size-5" aria-hidden="true" />
          {deleteNode.isPending
            ? t('neverConnected.cleaningUp')
            : t('neverConnected.cleanUp')}
        </Button>
        {deleteNode.isError ? (
          <span role="alert" className="text-destructive text-sm">
            {deleteNode.error.message}
          </span>
        ) : null}
      </div>
    </Alert>
  )
}
