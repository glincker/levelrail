import { useTranslation } from 'react-i18next'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
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
import {
  msUntilExpiry,
  formatCountdown,
  requesterLabel,
} from '../../lib/deviceLogin'
import {
  useApproveDeviceAuthRequest,
  type DeviceAuthRequest,
} from '../../queries/deviceAuth'
import { useNow } from '../../hooks/useNow'

// Approval is never one click: the operator sees the code large and must
// confirm it matches the terminal before anything is granted.
export function DeviceLoginConfirmDialog({
  request,
  open,
  onOpenChange,
}: {
  request: DeviceAuthRequest
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('attention', { useSuspense: false })
  const approve = useApproveDeviceAuthRequest()
  const now = useNow()
  const remaining = msUntilExpiry(request, now)
  const expired = remaining <= 0
  const name = requesterLabel(request) || t('deviceLogin.unknownDevice')

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('deviceLogin.confirmTitle')}</DialogTitle>
          <DialogDescription>{t('deviceLogin.confirmBody')}</DialogDescription>
        </DialogHeader>
        <div className="rounded-lg border border-border bg-muted/50 p-4 text-center">
          <p className="text-xs text-muted-foreground">
            {t('deviceLogin.codeLabel')}
          </p>
          <p
            className="font-mono text-4xl font-semibold tracking-widest"
            data-testid="device-confirm-code"
          >
            {request.user_code}
          </p>
          <p className="mt-2 text-xs text-muted-foreground">
            {t('deviceLogin.device', { name })}
            {request.requester_ip
              ? `, ${t('deviceLogin.from', { ip: request.requester_ip })}`
              : ''}
          </p>
          <p className="text-xs text-muted-foreground">
            {expired
              ? t('deviceLogin.expired')
              : t('deviceLogin.expiresIn', {
                  time: formatCountdown(remaining),
                })}
          </p>
        </div>
        {request.ip_mismatch ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>
              {t('deviceLogin.ipMismatch', { ip: request.requester_ip })}
            </AlertDescription>
          </Alert>
        ) : null}
        <p className="text-xs text-muted-foreground">
          {t('deviceLogin.agentHint')}
        </p>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              onOpenChange(false)
            }}
          >
            {t('deviceLogin.cancel')}
          </Button>
          <Button
            type="button"
            disabled={expired || approve.isPending}
            onClick={() => {
              approve.mutate(request.user_code, {
                onSuccess: () => {
                  toast.add({
                    title: t('deviceLogin.approved', { name }),
                    type: 'success',
                  })
                  onOpenChange(false)
                },
                onError: (error) => {
                  toast.add({ title: error.message, type: 'error' })
                },
              })
            }}
          >
            {t('deviceLogin.confirmApprove')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
