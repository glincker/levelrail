import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowsClockwiseIcon,
  EyeIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import { ApiError } from '@/lib/apiError'
import {
  useDeleteExternalDatabase,
  useProbeExternalDatabase,
  useRevealExternalPassword,
} from '../queries/externalDatabases'
import type { ExternalDatabase } from '../types/externalDatabase'
import { ExternalHealthBadge } from './ExternalHealthBadge'

const CONFLICT_STATUS = 409

function Detail({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div>
      <dt className="text-xs text-muted-foreground uppercase">{label}</dt>
      <dd className="mt-1 font-mono text-sm break-all text-foreground">
        {children}
      </dd>
    </div>
  )
}

// Removing an external database only deletes Levelrail's record and stored
// password; the copy says so because the managed delete dialog says the
// opposite about containers and volumes.
export function DeleteExternalDatabaseDialog({ name }: { name: string }) {
  const { t } = useTranslation('databases')
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()
  const remove = useDeleteExternalDatabase()
  const inUse =
    remove.error instanceof ApiError && remove.error.status === CONFLICT_STATUS

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) {
          remove.reset()
        }
      }}
    >
      <DialogTrigger render={<Button variant="destructive" size="sm" />}>
        <TrashIcon className="size-3.5" aria-hidden="true" />
        {t('external.delete.trigger')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{t('external.delete.title', { name })}</DialogTitle>
          <DialogDescription>
            {t('external.delete.description')}
          </DialogDescription>
        </DialogHeader>
        {remove.isError ? (
          <p className="text-sm text-destructive" role="alert">
            {remove.error.message}
          </p>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setOpen(false)
            }}
          >
            {t('delete.cancel')}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={remove.isPending}
            onClick={() => {
              remove.mutate(
                { name, force: inUse },
                {
                  onSuccess: () => {
                    setOpen(false)
                    toast.add({
                      title: t('external.delete.toast', { name }),
                      type: 'success',
                    })
                    void navigate({ to: '/databases' })
                  },
                },
              )
            }}
          >
            {remove.isPending
              ? t('external.delete.removing')
              : inUse
                ? t('external.delete.confirmAnyway')
                : t('external.delete.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function ExternalDatabaseOverview({ db }: { db: ExternalDatabase }) {
  const { t } = useTranslation('databases')
  const probe = useProbeExternalDatabase()
  const reveal = useRevealExternalPassword()
  const [shown, setShown] = useState<string | null>(null)
  const status = db.health?.status ?? 'unknown'

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('external.overview.title')}</CardTitle>
        <p className="text-sm text-muted-foreground">
          {t('external.overview.description')}
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-center gap-3">
          <ExternalHealthBadge status={status} />
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={probe.isPending}
            onClick={() => {
              probe.mutate(db.name, {
                onSuccess: (r) => {
                  toast.add({
                    title: t('external.overview.recheckedToast', {
                      status: t(`external.health.${r.status}`),
                    }),
                    type: r.status === 'reachable' ? 'success' : 'error',
                  })
                },
                onError: (e) => {
                  toast.add({ title: e.message, type: 'error' })
                },
              })
            }}
          >
            <ArrowsClockwiseIcon className="size-3.5" aria-hidden="true" />
            {probe.isPending
              ? t('external.overview.rechecking')
              : t('external.overview.recheck')}
          </Button>
          <span className="text-xs text-muted-foreground">
            {db.health?.checked_at
              ? t('external.overview.lastChecked', {
                  when: new Date(db.health.checked_at).toLocaleString(),
                })
              : t('external.overview.neverChecked')}
          </span>
        </div>
        <p className="text-sm text-muted-foreground">
          {t(`external.health.next.${status}`)}
          {db.health?.reason ? (
            <span className="mt-1 block font-mono text-xs break-words">
              {db.health.reason}
            </span>
          ) : null}
        </p>

        <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3">
          <Detail label={t('external.overview.engine')}>{db.engine}</Detail>
          <Detail label={t('external.overview.address')}>
            {db.host}:{db.port}
          </Detail>
          <Detail label={t('external.overview.tls')}>{db.tls_mode}</Detail>
          {db.username ? (
            <Detail label={t('external.overview.user')}>{db.username}</Detail>
          ) : null}
          {db.database ? (
            <Detail label={t('external.overview.database')}>
              {db.database}
            </Detail>
          ) : null}
          {db.network ? (
            <Detail label={t('external.overview.network')}>{db.network}</Detail>
          ) : null}
          {db.source_container ? (
            <Detail label={t('external.overview.source')}>
              {db.source_container}
            </Detail>
          ) : null}
          <Detail label={t('external.overview.password')}>
            {db.has_password ? (
              <span className="flex flex-wrap items-center gap-2">
                {shown ?? t('external.overview.passwordStored')}
                <Button
                  type="button"
                  size="xs"
                  variant="ghost"
                  onClick={() => {
                    if (shown !== null) {
                      setShown(null)
                      return
                    }
                    reveal.mutate(db.name, {
                      onSuccess: (r) => {
                        setShown(r.password)
                      },
                      onError: (e) => {
                        toast.add({ title: e.message, type: 'error' })
                      },
                    })
                  }}
                >
                  <EyeIcon className="size-3.5" aria-hidden="true" />
                  {shown !== null
                    ? t('external.overview.hide')
                    : t('external.overview.reveal')}
                </Button>
              </span>
            ) : (
              t('external.overview.passwordNone')
            )}
          </Detail>
        </dl>
        <p className="text-xs text-muted-foreground">
          {t('external.overview.usedBy', { ref: `${db.name}.url` })}{' '}
          {t('external.overview.revealNote')}
        </p>
      </CardContent>
    </Card>
  )
}
