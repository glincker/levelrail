import { ListSkeleton } from '@/components/ui/list-skeleton'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { EmptyState } from '@/components/ui/empty-state'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ReactNode } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { PackageGlassIcon, SearchGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { useAppListOptional } from '../queries/apps'

const MAX_VISIBLE_APPS = 50

// Picks the app to configure, then hands off to that app's Load balancer tab,
// which owns the actual form.
export function ConfigureLoadBalancerDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const navigate = useNavigate()
  const { t } = useTranslation('statePlaceholders')
  const apps = useAppListOptional()
  const [search, setSearch] = useState('')

  const matches = useMemo(() => {
    const needle = search.trim().toLowerCase()
    const all = apps.data ?? []
    return all.filter((a) => a.name.toLowerCase().includes(needle))
  }, [apps.data, search])

  const pick = (name: string) => {
    onOpenChange(false)
    void navigate({ to: '/apps/$name/loadbalancer', params: { name } })
  }

  let body: ReactNode
  if (apps.isPending) {
    body = <ListSkeleton />
  } else if (apps.isError) {
    body = (
      <EmptyState
        kind="error"
        icon={<TriangleAlertGlassIcon className="size-5" aria-hidden="true" />}
        title={t('loadBalancerPicker.loadError')}
        description={t('loadBalancerPicker.loadErrorHint')}
      />
    )
  } else if ((apps.data ?? []).length === 0) {
    body = (
      <EmptyState
        kind="first-run"
        icon={<PackageGlassIcon className="size-5" aria-hidden="true" />}
        title={t('lb.picker.noApps')}
        description={t('lb.picker.noAppsHint')}
        action={
          <Button
            render={<Link to="/apps" />}
            nativeButton={false}
            onClick={() => {
              onOpenChange(false)
            }}
          >
            {t('lb.picker.createApp')}
          </Button>
        }
      />
    )
  } else {
    body = (
      <div className="space-y-3">
        <Input
          type="search"
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
          }}
          placeholder={t('lb.picker.search')}
          aria-label={t('lb.picker.search')}
          autoFocus
        />
        {matches.length === 0 ? (
          <EmptyState
            kind="no-results"
            icon={<SearchGlassIcon className="size-5" aria-hidden="true" />}
            title={t('lb.picker.noMatch')}
            description={t('lb.picker.noMatchHint')}
          />
        ) : (
          <ul className="max-h-64 space-y-1 overflow-auto">
            {matches.slice(0, MAX_VISIBLE_APPS).map((app) => (
              <li key={app.name}>
                <button
                  type="button"
                  className="flex min-h-(--glinr-control-h) w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted focus-visible:bg-muted focus-visible:outline-none"
                  onClick={() => {
                    pick(app.name)
                  }}
                >
                  <PackageGlassIcon
                    className="size-5 text-muted-foreground"
                    aria-hidden="true"
                  />
                  <span className="truncate">{app.name}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
        {matches.length > MAX_VISIBLE_APPS ? (
          <p className="text-xs text-muted-foreground">
            {t('lb.picker.truncated', { count: MAX_VISIBLE_APPS })}
          </p>
        ) : null}
      </div>
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('lb.picker.title')}</DialogTitle>
          <DialogDescription>{t('lb.picker.description')}</DialogDescription>
        </DialogHeader>
        {body}
      </DialogContent>
    </Dialog>
  )
}
