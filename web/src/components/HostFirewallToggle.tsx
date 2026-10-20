import { useState } from 'react'
import { useTranslation } from 'react-i18next'
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
import { useHostFirewall, useSetHostFirewall } from '../queries/hostFirewall'

/** HostFirewallToggle is the one place the firewall is switched: it previews the exact commands and only runs them after Confirm. */
export function HostFirewallToggle() {
  const { t } = useTranslation('settings')
  const { data } = useHostFirewall()
  const setFirewall = useSetHostFirewall()
  const [confirming, setConfirming] = useState(false)

  if (!data?.installed) return null
  const turningOn = !data.active
  const commands = turningOn ? data.commands : ['ufw disable']

  function apply() {
    setFirewall.mutate(
      { enable: turningOn },
      {
        onSuccess: () => {
          setConfirming(false)
          toast.add({
            title: t(
              turningOn
                ? 'hostFirewall.enabledToast'
                : 'hostFirewall.disabledToast',
            ),
            type: 'success',
          })
        },
        onError: (err) => {
          toast.add({
            title: t('hostFirewall.failedToast'),
            description: err.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <>
      <Button
        variant={turningOn ? 'default' : 'outline'}
        size="sm"
        onClick={() => setConfirming(true)}
      >
        {t(turningOn ? 'hostFirewall.turnOn' : 'hostFirewall.turnOff')}
      </Button>
      <Dialog open={confirming} onOpenChange={setConfirming}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>
              {t(
                turningOn
                  ? 'hostFirewall.confirmOnTitle'
                  : 'hostFirewall.confirmOffTitle',
              )}
            </DialogTitle>
            <DialogDescription>
              {t(
                turningOn
                  ? 'hostFirewall.confirmOnBody'
                  : 'hostFirewall.confirmOffBody',
              )}
            </DialogDescription>
          </DialogHeader>
          <pre className="max-h-48 overflow-auto rounded-md bg-muted p-3 font-mono text-xs">
            {commands.join('\n')}
          </pre>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirming(false)}>
              {t('hostFirewall.cancel')}
            </Button>
            <Button onClick={apply} disabled={setFirewall.isPending}>
              {t('hostFirewall.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
