import { useRouter } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CirclePauseGlassIcon,
  CountdownGlassIcon,
  RefreshCwGlassIcon,
} from '@thesvg/ui-react'
import { StatusPill } from '@/components/kit/StatusPill'
import type { Tone } from '@/components/kit/tone'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatAge } from '../../lib/format'
import type { NodeResource, NodeStatus } from '../../types/nodeDetail'
import { ReenrollNodeDialog } from '../ReenrollNodeDialog'

const TONE_BY_STATUS: Record<NodeStatus, Tone> = {
  pending: 'neutral',
  online: 'success',
  offline: 'danger',
  cordoned: 'warning',
}

const ICON_BY_STATUS = {
  pending: CountdownGlassIcon,
  online: CircleCheckGlassIcon,
  offline: CircleAlertGlassIcon,
  cordoned: CirclePauseGlassIcon,
} as const

// An unhealthy node gets the fix as the primary button (re-enroll its agent),
// with Re-check beside it, instead of a paragraph of instructions.
export function NodeStatusCard({ node }: { node: NodeResource }) {
  const { t } = useTranslation('nodes')
  const router = useRouter()
  const Icon = ICON_BY_STATUS[node.status]
  const unhealthy = node.status === 'offline'
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('detail.statusCard.title')}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-wrap items-center gap-3">
        <StatusPill
          tone={TONE_BY_STATUS[node.status]}
          label={t(`status.${node.status}`)}
          icon={<Icon aria-hidden="true" />}
        />
        <span className="min-w-0 flex-1 basis-48 text-sm text-muted-foreground">
          {t(`detail.statusCard.${node.status}`)}
          {' · '}
          {t('detail.statusCard.lastSeen', {
            when: formatAge(node.last_seen_at, t('detail.overview.never')),
          })}
        </span>
        <div className="flex flex-wrap items-center gap-2">
          {unhealthy ? <ReenrollNodeDialog node={node} /> : null}
          {node.status === 'online' ? null : (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                void router.invalidate()
              }}
            >
              <RefreshCwGlassIcon size="1.2em" aria-hidden="true" />
              {t('detail.statusCard.recheck')}
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
