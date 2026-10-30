import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CopyGlassIcon,
  DownloadGlassIcon,
  KeyGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
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
import { useCopyToClipboard } from '../hooks/useCopyToClipboard'
import {
  useGenerateControlPlaneDrKeypair,
  type ControlPlaneDrKeypair,
} from '../queries/controlPlaneDr'

function saveTextFile(name: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/plain' }))
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}

export function ControlPlaneDrKeygen({
  onUseRecipient,
}: {
  onUseRecipient: (recipient: string) => void
}) {
  const { t } = useTranslation('settings')
  const generate = useGenerateControlPlaneDrKeypair()
  const [pair, setPair] = useState<ControlPlaneDrKeypair | null>(null)
  const [saved, setSaved] = useState(false)
  const { copied, copy } = useCopyToClipboard()

  function close() {
    setPair(null)
    setSaved(false)
  }

  return (
    <>
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={generate.isPending}
        onClick={() => {
          generate.mutate(undefined, {
            onSuccess: (kp) => {
              setPair(kp)
            },
            onError: (e) => {
              toast.add({ title: e.message, type: 'error' })
            },
          })
        }}
      >
        <KeyGlassIcon className="size-5" aria-hidden="true" />
        {t('controlPlaneDrKeygen.trigger')}
      </Button>
      <Dialog
        open={pair !== null}
        onOpenChange={(next) => {
          if (!next) close()
        }}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t('controlPlaneDrKeygen.title')}</DialogTitle>
            <DialogDescription>
              {t('controlPlaneDrKeygen.description')}
            </DialogDescription>
          </DialogHeader>
          {pair !== null ? (
            <div className="space-y-3">
              <Alert>
                <TriangleAlertGlassIcon
                  className="size-6 text-tone-warning"
                  aria-hidden="true"
                />
                <AlertDescription>
                  {t('controlPlaneDrKeygen.warning')}
                </AlertDescription>
              </Alert>
              <pre className="max-h-40 overflow-auto rounded-md bg-muted p-3 font-mono text-xs break-all whitespace-pre-wrap">
                {pair.identity}
              </pre>
              <div className="flex flex-wrap gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    copy(pair.identity)
                  }}
                >
                  {copied ? (
                    <CheckGlassIcon className="size-5" aria-hidden="true" />
                  ) : (
                    <CopyGlassIcon className="size-5" aria-hidden="true" />
                  )}
                  {t('controlPlaneDrKeygen.copy')}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    saveTextFile(
                      'backup-recipient-private-key.txt',
                      pair.identity,
                    )
                    setSaved(true)
                  }}
                >
                  <DownloadGlassIcon className="size-5" aria-hidden="true" />
                  {t('controlPlaneDrKeygen.download')}
                </Button>
              </div>
            </div>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              disabled={pair === null || (!saved && !copied)}
              onClick={() => {
                if (pair !== null) onUseRecipient(pair.recipient)
                close()
              }}
            >
              {t('controlPlaneDrKeygen.use')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
