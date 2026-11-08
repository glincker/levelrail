import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CircleQuestionMarkGlassIcon,
  TriangleAlertGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import { useNodePatchStatus } from '../queries/nodes'
import type { NodePatchStatusResource } from '../types/nodeDetail'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { RoleGate } from '@/components/ui/RoleGate'
import { Skeleton } from '@/components/ui/skeleton'
import type { VariantProps } from 'class-variance-authority'

const NodePatchDialog = lazy(() => import('./nodes/NodePatchDialog'))
const NodePatchRuns = lazy(() => import('./nodes/NodePatchRuns'))

type PatchState = 'loading' | 'unknown' | 'up-to-date' | 'updates' | 'security'

type PatchStatusConfig = {
  icon: UiIcon
  variant: VariantProps<typeof badgeVariants>['variant']
  labelKey:
    'patch.checking' | 'patch.unknown' | 'patch.upToDate' | 'patch.updates'
}

function resolvePatchState(
  isPending: boolean,
  isError: boolean,
  data: NodePatchStatusResource | undefined,
): PatchState {
  if (isPending) return 'loading'
  if (isError || !data?.checked) return 'unknown'
  if (data.total === 0) return 'up-to-date'
  if (data.security > 0) return 'security'
  return 'updates'
}

// GET /api/v1/nodes/{id}/patch-status (internal/api/node_patch_status.go):
// one current fact ("3 updates available, 1 security"), read from the latest
// sample rather than the generic metrics time-series.
function patchStatusConfig(state: PatchState): PatchStatusConfig {
  switch (state) {
    case 'loading':
      return {
        icon: CircleQuestionMarkGlassIcon,
        variant: 'muted',
        labelKey: 'patch.checking',
      }
    case 'unknown':
      return {
        icon: CircleQuestionMarkGlassIcon,
        variant: 'muted',
        labelKey: 'patch.unknown',
      }
    case 'up-to-date':
      return {
        icon: CircleCheckGlassIcon,
        variant: 'success',
        labelKey: 'patch.upToDate',
      }
    case 'security':
      return {
        icon: CircleAlertGlassIcon,
        variant: 'destructive',
        labelKey: 'patch.updates',
      }
    default:
      return {
        icon: TriangleAlertGlassIcon,
        variant: 'warning',
        labelKey: 'patch.updates',
      }
  }
}

interface Props {
  nodeId: string
  nodeName: string
  isLocal: boolean
}

export function NodePatchStatusCard({
  nodeId,
  nodeName,
  isLocal,
}: Readonly<Props>) {
  const { t } = useTranslation('nodes')
  const { data, isPending, isError } = useNodePatchStatus(nodeId)
  const [previewOpen, setPreviewOpen] = useState(false)
  const state = resolvePatchState(isPending, isError, data)
  const { icon: StatusIcon, variant, labelKey } = patchStatusConfig(state)
  const count = data?.checked ? data.total : 0

  return (
    <Card id="node-patching">
      <CardHeader>
        <CardTitle>{t('patch.title')}</CardTitle>
        <CardDescription>{t('patch.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={variant}>
            <StatusIcon className="size-3.5" />
            {t(labelKey, { count })}
          </Badge>
          {state === 'security' && data ? (
            <Badge variant="destructive">
              {t('patch.security', { count: data.security })}
            </Badge>
          ) : null}
          {data?.checked_at ? (
            <span className="text-xs text-muted-foreground">
              {t('patch.checkedAt', {
                time: new Date(data.checked_at).toLocaleString(),
              })}
            </span>
          ) : null}
          {isLocal ? (
            <span className="ml-auto">
              <RoleGate ability="root">
                <Button
                  type="button"
                  size="sm"
                  onClick={() => setPreviewOpen(true)}
                >
                  {t('patch.preview')}
                </Button>
              </RoleGate>
            </span>
          ) : null}
        </div>
        {isLocal ? (
          <Suspense fallback={<Skeleton className="h-10 w-full" />}>
            <NodePatchRuns nodeId={nodeId} />
          </Suspense>
        ) : (
          <p className="text-xs text-muted-foreground">
            {t('patch.remoteNote')}
          </p>
        )}
      </CardContent>
      {previewOpen ? (
        <Suspense fallback={null}>
          <NodePatchDialog
            nodeId={nodeId}
            nodeName={nodeName}
            onClose={() => setPreviewOpen(false)}
          />
        </Suspense>
      ) : null}
    </Card>
  )
}
