import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { PackageIcon } from '@phosphor-icons/react/dist/ssr'
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
    body = <p className="text-sm text-muted-foreground">Loading apps...</p>
  } else if (apps.isError) {
    body = (
      <p className="text-sm text-destructive">
        Could not load your apps. Close this dialog and try again.
      </p>
    )
  } else if ((apps.data ?? []).length === 0) {
    body = (
      <div className="space-y-3">
        <p className="text-sm text-muted-foreground">
          You have no apps yet. A load balancer spreads traffic across an app's
          replicas, so create an app first.
        </p>
        <Button
          size="sm"
          render={<Link to="/apps" />}
          nativeButton={false}
          onClick={() => {
            onOpenChange(false)
          }}
        >
          Create an app
        </Button>
      </div>
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
          placeholder="Search apps"
          aria-label="Search apps"
          autoFocus
        />
        {matches.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No app matches that search.
          </p>
        ) : (
          <ul className="max-h-64 space-y-1 overflow-auto">
            {matches.slice(0, MAX_VISIBLE_APPS).map((app) => (
              <li key={app.name}>
                <button
                  type="button"
                  className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted focus-visible:bg-muted focus-visible:outline-none"
                  onClick={() => {
                    pick(app.name)
                  }}
                >
                  <PackageIcon
                    className="size-4 text-muted-foreground"
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
            Showing the first {MAX_VISIBLE_APPS} matches. Refine the search to
            narrow it down.
          </p>
        ) : null}
      </div>
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Configure a load balancer</DialogTitle>
          <DialogDescription>
            Choose the app whose replicas you want to balance.
          </DialogDescription>
        </DialogHeader>
        {body}
      </DialogContent>
    </Dialog>
  )
}
