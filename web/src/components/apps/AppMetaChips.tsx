import { GlobeIcon, PackageIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { TemplateLogo } from '../TemplateLogo'
import { logoIdForImage } from '../../lib/imageLogo'
import type { AppListEntry } from '../../types/appDetail'

export function AppLogo({ image }: { image: string }) {
  return (
    <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
      <TemplateLogo
        id={logoIdForImage(image) ?? ''}
        className="size-4"
        fallback={<PackageIcon className="size-4" aria-hidden="true" />}
      />
    </span>
  )
}

export function AppMetaChips({ app }: { app: AppListEntry }) {
  const domain = app.domains?.[0]
  const extra = (app.domains?.length ?? 0) - 1
  return (
    <span className="flex min-w-0 flex-wrap items-center gap-1">
      {app.environment_name ? (
        <Badge variant="muted" className="px-1.5 py-0 text-[10px]">
          {app.environment_name}
        </Badge>
      ) : null}
      {domain ? (
        <Badge
          variant="outline"
          className="max-w-44 gap-1 px-1.5 py-0 text-[10px] text-muted-foreground"
        >
          <GlobeIcon className="size-3 shrink-0" aria-hidden="true" />
          <span className="truncate">{domain}</span>
          {extra > 0 ? <span>+{extra}</span> : null}
        </Badge>
      ) : null}
    </span>
  )
}
