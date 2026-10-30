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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useDeleteDnsRecord } from '@/queries/dns'
import type { DnsRecordSet } from '@/types/dns'

/** Typed-confirm delete; a 409 for a Levelrail written record asks once more, then retries with force. */
export function RecordDeleteDialog({
  zone,
  record,
  onClose,
}: {
  zone: string
  record: DnsRecordSet
  onClose: () => void
}) {
  const { t } = useTranslation('dnsProviders')
  const del = useDeleteDnsRecord(zone)
  const [typed, setTyped] = useState('')
  const [needForce, setNeedForce] = useState(false)
  const [error, setError] = useState('')

  function submit(force: boolean) {
    setError('')
    del.mutate(
      {
        name: record.name,
        type: record.type,
        set_identifier: record.set_identifier,
        force,
      },
      {
        onSuccess: onClose,
        onError: (err) => {
          if (err.status === 409 && !force) setNeedForce(true)
          else setError(err.message || t('records.failed'))
        },
      },
    )
  }

  return (
    <Dialog open onOpenChange={(o) => (o ? undefined : onClose())}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {needForce
              ? t('records.managedTitle')
              : t('records.deleteTitle', {
                  name: record.name,
                  type: record.type,
                })}
          </DialogTitle>
          <DialogDescription>
            {needForce ? t('records.managedBody') : t('records.deleteLead')}
          </DialogDescription>
        </DialogHeader>
        {needForce ? null : (
          <div className="space-y-1.5">
            <Label htmlFor="dnsp-record-confirm">
              {t('records.typeName', { name: record.name })}
            </Label>
            <Input
              id="dnsp-record-confirm"
              className="font-mono"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
            />
          </div>
        )}
        {error ? (
          <p role="alert" className="text-sm text-tone-danger">
            {error}
          </p>
        ) : null}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('records.cancel')}
          </Button>
          <Button
            variant="destructive"
            disabled={del.isPending || (!needForce && typed !== record.name)}
            onClick={() => submit(needForce)}
          >
            {needForce ? t('records.forceConfirm') : t('records.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
