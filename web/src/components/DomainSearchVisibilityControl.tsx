import {
  EyeSlashIcon,
  EyeIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  useDomainSearchVisibility,
  useSetDomainSearchVisibility,
} from '../queries/domainSearchVisibility'

export function DomainSearchVisibilityControl({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { t } = useTranslation('domains')
  const { data, isLoading } = useDomainSearchVisibility(appName, domain)
  const set = useSetDomainSearchVisibility(appName, domain)

  if (isLoading) {
    return (
      <div
        className="flex items-center justify-between gap-2 rounded-md border border-border bg-muted/30 p-3"
        aria-hidden="true"
      >
        <Skeleton className="h-5 w-28 rounded-full" />
        <Skeleton className="h-7 w-32" />
      </div>
    )
  }

  const hidden = data?.hidden ?? false
  return (
    <div className="rounded-md border border-border bg-muted/30 p-3 text-sm">
      <div className="flex items-center justify-between gap-2">
        {hidden ? (
          <Badge variant="warning" className="shrink-0">
            <EyeSlashIcon className="size-3" />
            {t('searchVisibility.hiddenBadge')}
          </Badge>
        ) : (
          <Badge variant="muted" className="shrink-0">
            <EyeIcon className="size-3" />
            {t('searchVisibility.visibleBadge')}
          </Badge>
        )}
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={set.isPending}
          onClick={() => {
            set.mutate(!hidden)
          }}
        >
          {hidden ? t('searchVisibility.show') : t('searchVisibility.hide')}
        </Button>
      </div>
      <p className="mt-2 text-xs text-muted-foreground">
        {hidden
          ? t('searchVisibility.hiddenHelp', { domain })
          : t('searchVisibility.visibleHelp')}
      </p>
      {set.isError ? (
        <Alert variant="destructive" className="mt-2">
          <WarningIcon />
          <AlertDescription>{set.error.message}</AlertDescription>
        </Alert>
      ) : null}
    </div>
  )
}
