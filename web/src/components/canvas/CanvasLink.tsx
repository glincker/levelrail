import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { LayoutGridGlassIcon } from '@thesvg/ui-react'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { useExperimentalFeatures } from '../../hooks/useExperimental'
import { isFeatureVisible } from '../../lib/experimental'

// Gate first, translate inside: with the beta off, the canvas strings are never fetched.
export function CanvasLink({ projectId }: { projectId: string }) {
  const enabled = useExperimentalFeatures()
  if (!isFeatureVisible('canvas', enabled)) return null
  return <CanvasLinkButton projectId={projectId} />
}

function CanvasLinkButton({ projectId }: { projectId: string }) {
  const { t } = useTranslation('canvas')
  return (
    <Link
      to="/projects/$id/canvas"
      params={{ id: projectId }}
      className={cn(buttonVariants({ variant: 'outline', size: 'sm' }))}
    >
      <LayoutGridGlassIcon size="1.2em" aria-hidden="true" />
      {t('link')}
    </Link>
  )
}
