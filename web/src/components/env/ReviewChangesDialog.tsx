import { useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { EnvChange } from '../../lib/envRows'
import { useScanEnvValues } from '../../queries/envScan'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

export interface ReviewChangesDialogProps {
  open: boolean
  changes: EnvChange[]
  saving: boolean
  error: string | null
  scanApp: string
  scanVars: Record<string, string>
  onStoreAsSecrets: (keys: string[]) => void
  onConfirm: () => void
  onOpenChange: (open: boolean) => void
}

const TONE = {
  added: 'success',
  changed: 'info',
  removed: 'destructive',
} as const

export function ReviewChangesDialog({
  open,
  changes,
  saving,
  error,
  scanApp,
  scanVars,
  onStoreAsSecrets,
  onConfirm,
  onOpenChange,
}: ReviewChangesDialogProps) {
  const { t } = useTranslation('envEditor')
  const scan = useScanEnvValues(scanApp)
  const scanKey = useMemo(() => JSON.stringify(scanVars), [scanVars])
  const { mutate: runScan, reset: resetScan } = scan
  useEffect(() => {
    if (!open || Object.keys(scanVars).length === 0) {
      resetScan()
      return
    }
    runScan(scanVars)
    // scanKey stands in for scanVars so an equal object does not rescan.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, scanKey, runScan, resetScan])
  const findings = scan.data ?? []
  const hidden = t('review.hiddenValue')
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('review.title')}</DialogTitle>
          <DialogDescription>{t('review.description')}</DialogDescription>
        </DialogHeader>
        {changes.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('review.none')}</p>
        ) : (
          <ul
            className="max-h-80 space-y-1.5 overflow-auto text-sm"
            aria-label={t('review.title')}
          >
            {changes.map((c) => (
              <li
                key={`${c.kind}-${c.key}`}
                className="flex flex-wrap items-center gap-2"
              >
                <Badge variant={TONE[c.kind]}>{t(`review.${c.kind}`)}</Badge>
                <span className="font-mono break-all">{c.key}</span>
                <span className="text-xs text-muted-foreground">
                  {t(`types.${c.type}`)}
                </span>
                <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
                  {c.kind === 'added'
                    ? (c.to ?? hidden)
                    : c.kind === 'removed'
                      ? (c.from ?? hidden)
                      : `${c.from ?? hidden} → ${c.to ?? hidden}`}
                </span>
              </li>
            ))}
          </ul>
        )}
        {findings.length > 0 ? (
          <Alert role="status">
            <AlertTitle>{t('review.scan.title')}</AlertTitle>
            <AlertDescription>
              <p>{t('review.scan.description')}</p>
              <ul className="mt-1 list-disc pl-4">
                {findings.map((f) => (
                  <li key={`${f.var}-${f.fingerprint}`}>
                    {t('review.scan.finding', {
                      key: f.var,
                      rule: f.description,
                    })}
                  </li>
                ))}
              </ul>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="mt-2"
                onClick={() => {
                  onStoreAsSecrets(findings.map((f) => f.var))
                }}
              >
                {t('review.scan.makeSecret')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {error ? (
          <Alert variant="destructive" role="alert">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="ghost"
            onClick={() => {
              onOpenChange(false)
            }}
          >
            {t('review.back')}
          </Button>
          <Button
            type="button"
            writeAction
            disabled={saving || changes.length === 0}
            onClick={onConfirm}
          >
            {saving ? t('review.saving') : t('review.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
