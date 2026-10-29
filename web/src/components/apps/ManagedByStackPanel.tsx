import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  BookOpenGlassIcon,
  LayersGlassIcon,
  LockGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { useExperimentalFeatures } from '../../hooks/useExperimental'
import { useStackByName } from '../../queries/stacks'
import { RevisionStatusBadge } from '../stacks/StackIndicators'

function Panel({ stack }: Readonly<{ stack: string }>) {
  const { t } = useTranslation('stacks')
  const { data } = useStackByName(stack)
  return (
    <Card>
      <CardContent className="flex flex-wrap items-center gap-x-3 gap-y-2 py-2 text-sm">
        <LayersGlassIcon
          className="size-5 text-muted-foreground"
          aria-hidden="true"
        />
        <span className="font-medium text-foreground">
          {t('appPanel.title')}{' '}
          <Link
            to="/settings/stacks"
            search={{ stack }}
            className="underline-offset-2 hover:underline"
          >
            {stack}
          </Link>
        </span>
        {data && data.latest_revision > 0 ? (
          <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
            {t('appPanel.revision', { number: data.latest_revision })}
            <RevisionStatusBadge status={data.latest_status} />
          </span>
        ) : null}
        {data?.drift_policy === 'lock' ? (
          <Badge variant="warning">
            <LockGlassIcon size="1.4em" aria-hidden="true" />
            {t('appPanel.lock')}
          </Badge>
        ) : null}
        <span className="ml-auto inline-flex items-center gap-1.5 text-xs text-muted-foreground">
          {t('appPanel.hint')}
          <Link
            to="/help/$"
            params={{ _splat: 'stacks' }}
            className="inline-flex items-center gap-1 text-foreground underline-offset-2 hover:underline"
          >
            <BookOpenGlassIcon className="size-5" aria-hidden="true" />
            {t('appPanel.docs')}
          </Link>
        </span>
      </CardContent>
    </Card>
  )
}

// Compact "managed by" strip for the app overview; nothing for unmanaged apps.
export function ManagedByStackPanel({
  stack,
}: Readonly<{ stack: string | undefined }>) {
  const iacOn = useExperimentalFeatures().includes('iac')
  return stack && iacOn ? <Panel stack={stack} /> : null
}
