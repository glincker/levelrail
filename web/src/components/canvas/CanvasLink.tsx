import { useId } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { LayoutGridGlassIcon } from '@thesvg/ui-react'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { useExperimentalFeatures } from '../../hooks/useExperimental'
import {
  BETA_FEATURES_PATH,
  isFeatureVisible,
  lockedSearch,
} from '../../lib/experimental'
import { LockedFeatureMark } from '../shell/LockedFeatureMark'

// With the beta off the link stays visible, locked, and leads to Beta features.
export function CanvasLink({ projectId }: { projectId: string }) {
  const enabled = useExperimentalFeatures()
  if (!isFeatureVisible('canvas', enabled)) return <LockedCanvasLink />
  return <CanvasLinkButton projectId={projectId} />
}

function LockedCanvasLink() {
  const { t } = useTranslation('canvas')
  const describedBy = useId()
  return (
    <Link
      to={BETA_FEATURES_PATH}
      search={lockedSearch('canvas')}
      aria-describedby={describedBy}
      data-locked="true"
      className={cn(
        buttonVariants({ variant: 'outline', size: 'sm' }),
        'text-muted-foreground',
      )}
    >
      <LayoutGridGlassIcon size="1.4em" aria-hidden="true" />
      {t('link')}
      <LockedFeatureMark id={describedBy} />
    </Link>
  )
}

function CanvasLinkButton({ projectId }: { projectId: string }) {
  const { t } = useTranslation('canvas')
  return (
    <Link
      to="/projects/$id/canvas"
      params={{ id: projectId }}
      className={cn(buttonVariants({ variant: 'outline', size: 'sm' }))}
    >
      <LayoutGridGlassIcon size="1.4em" aria-hidden="true" />
      {t('link')}
    </Link>
  )
}
