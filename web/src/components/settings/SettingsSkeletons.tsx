import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

export function SettingsHeaderSkeleton({ icon = false }: { icon?: boolean }) {
  if (!icon) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-5 w-40" />
        <Skeleton className="h-4 w-64" />
      </div>
    )
  }
  return (
    <div className="flex items-start gap-3">
      <Skeleton className="mt-0.5 size-8 shrink-0 rounded-lg" />
      <div className="space-y-2">
        <Skeleton className="h-5 w-40" />
        <Skeleton className="h-4 w-64" />
      </div>
    </div>
  )
}

export type SettingsCardRowVariant = 'field' | 'line' | 'list' | 'status'

// Mirrors settings pages' own real shape: a Card with a title/description
// header (optionally an icon box, optionally a header action button) and
// N body rows, so this covers the single-card-form pages, and composes
// into the multi-card ones, without every route re-declaring the same
// Card/Skeleton JSX.
export function SettingsCardSkeleton({
  description = true,
  headerIcon = false,
  headerAction = false,
  rows = 2,
  rowVariant = 'field',
}: {
  description?: boolean
  headerIcon?: boolean
  headerAction?: boolean
  rows?: number
  rowVariant?: SettingsCardRowVariant
}) {
  return (
    <Card>
      <CardHeader>
        {headerAction ? (
          <div className="flex items-center justify-between gap-3">
            <SettingsCardTitleSkeleton
              icon={headerIcon}
              description={description}
            />
            <Skeleton className="h-8 w-24 rounded-md" />
          </div>
        ) : (
          <SettingsCardTitleSkeleton
            icon={headerIcon}
            description={description}
          />
        )}
      </CardHeader>
      {rows > 0 ? (
        <CardContent className="space-y-3">
          {Array.from({ length: rows }, (_, i) => (
            <SettingsCardRowSkeleton key={i} variant={rowVariant} />
          ))}
        </CardContent>
      ) : null}
    </Card>
  )
}

function SettingsCardTitleSkeleton({
  icon,
  description,
}: {
  icon: boolean
  description: boolean
}) {
  const title = (
    <div className="space-y-2">
      <Skeleton className="h-4 w-32" />
      {description ? <Skeleton className="h-3.5 w-56" /> : null}
    </div>
  )
  if (!icon) {
    return title
  }
  return (
    <div className="flex items-center gap-3">
      <Skeleton className="size-8 shrink-0 rounded-lg" />
      {title}
    </div>
  )
}

function SettingsCardRowSkeleton({
  variant,
}: {
  variant: SettingsCardRowVariant
}) {
  if (variant === 'field') {
    return (
      <div className="space-y-1.5">
        <Skeleton className="h-3 w-20" />
        <Skeleton className="h-9 w-full" />
      </div>
    )
  }
  if (variant === 'list') {
    return (
      <div className="flex items-center justify-between gap-3 py-1">
        <Skeleton className="h-4 w-40" />
        <Skeleton className="h-5 w-16 rounded-full" />
      </div>
    )
  }
  if (variant === 'status') {
    return (
      <div className="flex items-center justify-between gap-3 rounded-lg border border-border px-4 py-3">
        <div className="space-y-1.5">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-3.5 w-48" />
        </div>
        <Skeleton className="h-8 w-20 rounded-md" />
      </div>
    )
  }
  return <Skeleton className="h-4 w-full" />
}
