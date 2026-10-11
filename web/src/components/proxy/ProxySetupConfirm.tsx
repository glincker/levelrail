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

export function ProxySetupConfirm({
  open,
  onOpenChange,
  directory,
  domains,
  pending,
  onConfirm,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  directory: string
  domains: string[]
  pending: boolean
  onConfirm: () => void
}) {
  const { t } = useTranslation('domains')
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('proxySetup.confirm.title')}</DialogTitle>
          <DialogDescription>
            {directory
              ? t('proxySetup.confirm.directory', { dir: directory })
              : t('proxySetup.confirm.directoryUnknown')}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <p className="text-xs font-medium text-muted-foreground">
            {t('proxySetup.confirm.domainsLabel')}
          </p>
          {domains.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {t('proxySetup.confirm.noDomains')}
            </p>
          ) : (
            <ul className="max-h-40 space-y-1 overflow-y-auto rounded-md border border-border p-2 font-mono text-xs">
              {domains.map((d) => (
                <li key={d}>{d}</li>
              ))}
            </ul>
          )}
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t('proxySetup.confirm.cancel')}
          </Button>
          <Button type="button" disabled={pending} onClick={onConfirm}>
            {t('proxySetup.confirm.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
