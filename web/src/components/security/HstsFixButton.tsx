import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CircleAlertGlassIcon, CircleCheckGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'
import { hstsQuery, useSetHsts } from '../../queries/securityCenter'

export function HstsFixButton() {
  const { t } = useTranslation('security')
  const [open, setOpen] = useState(false)
  const probe = useQuery({ ...hstsQuery(), enabled: open })
  const setHsts = useSetHsts()
  const status = probe.data

  function enable() {
    setHsts.mutate(true, {
      onSuccess: () => {
        toast.add({ title: t('hsts.enabled'), type: 'success' })
        setOpen(false)
      },
      onError: (e) =>
        toast.add({ title: e.message || t('hsts.enableError'), type: 'error' }),
    })
  }

  return (
    <>
      <Button type="button" size="sm" onClick={() => setOpen(true)}>
        {t('actions.enable_hsts')}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t('hsts.title')}</DialogTitle>
            <DialogDescription>{t('hsts.description')}</DialogDescription>
          </DialogHeader>
          {probe.isPending ? (
            <p className="text-sm text-muted-foreground" role="status">
              {t('hsts.probing')}
            </p>
          ) : null}
          {probe.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{t('hsts.probeError')}</AlertDescription>
            </Alert>
          ) : null}
          {status ? (
            <div className="space-y-3">
              <ul className="space-y-1.5 text-sm">
                {status.hosts.map((h) => (
                  <li key={h.host} className="flex items-start gap-2">
                    {h.verified ? (
                      <CircleCheckGlassIcon
                        className="mt-0.5 size-5 shrink-0 text-green-600"
                        aria-hidden="true"
                      />
                    ) : (
                      <CircleAlertGlassIcon
                        className="mt-0.5 size-5 shrink-0 text-destructive"
                        aria-hidden="true"
                      />
                    )}
                    <span>
                      <span className="font-medium">{h.host}</span>
                      <span className="block text-muted-foreground">
                        {h.verified ? t('hsts.trusted') : t('hsts.notTrusted')}
                      </span>
                    </span>
                  </li>
                ))}
              </ul>
              {status.hosts.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  {t('hsts.noDomains')}
                </p>
              ) : null}
              {!status.ready ? (
                <Alert variant="destructive">
                  <AlertDescription>{t('hsts.blocked')}</AlertDescription>
                </Alert>
              ) : null}
              <Alert>
                <AlertDescription>{t('hsts.warning')}</AlertDescription>
              </Alert>
            </div>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => void probe.refetch()}
              disabled={probe.isFetching}
            >
              {t('hsts.recheck')}
            </Button>
            <Button
              type="button"
              onClick={enable}
              disabled={!status?.ready || setHsts.isPending}
            >
              {t('hsts.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
