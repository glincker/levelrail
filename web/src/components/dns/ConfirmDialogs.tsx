import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from '@tanstack/react-router'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import { toast } from '@/components/ui/toast'
import { useDeleteDnsRecord, useDeleteDnsZone } from '../../queries/dns'
import type { DnsRecordSet } from '../../types/dns'

/** DeleteZoneDialog needs the zone name typed, and force when records remain. */
export function DeleteZoneDialog({
  zone,
  zoneName,
  recordCount,
  onClose,
}: {
  zone: string
  zoneName: string
  recordCount: number
  onClose: () => void
}) {
  const { t } = useTranslation('dns')
  const navigate = useNavigate()
  const del = useDeleteDnsZone(zone)
  const [confirm, setConfirm] = useState('')
  const [force, setForce] = useState(false)
  const needsForce = recordCount > 0

  function submit() {
    del.mutate(
      { confirm, force },
      {
        onSuccess: () => {
          toast.add({
            title: t('zone.deletedToast', { zone: zoneName }),
            type: 'success',
          })
          onClose()
          void navigate({ to: '/dns' })
        },
        onError: (err) =>
          toast.add({
            title: t('zone.deleteFailed'),
            description: err.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <Dialog open onOpenChange={(o) => (o ? undefined : onClose())}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('zone.deleteTitle', { zone: zoneName })}</DialogTitle>
          <DialogDescription>
            {needsForce
              ? t('zone.deleteHasRecords', { count: recordCount })
              : t('zone.deleteLead')}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="dns-zone-confirm">
              {t('zone.typeName', { zone: zoneName })}
            </Label>
            <Input
              id="dns-zone-confirm"
              className="font-mono"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
            />
          </div>
          {needsForce ? (
            <div className="flex items-center gap-2">
              <Checkbox
                id="dns-zone-force"
                checked={force}
                onCheckedChange={(v) => setForce(v === true)}
              />
              <Label htmlFor="dns-zone-force">{t('zone.force')}</Label>
            </div>
          ) : null}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="destructive"
            disabled={
              confirm !== zoneName || (needsForce && !force) || del.isPending
            }
            onClick={submit}
          >
            {t('zone.delete')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function DeleteRecordDialog({
  zone,
  record,
  onClose,
}: {
  zone: string
  record: DnsRecordSet
  onClose: () => void
}) {
  const { t } = useTranslation('dns')
  const del = useDeleteDnsRecord(zone)
  return (
    <Dialog open onOpenChange={(o) => (o ? undefined : onClose())}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {t('records.deleteTitle', { name: record.name, type: record.type })}
          </DialogTitle>
          <DialogDescription>
            {t('records.deleteLead', { count: record.values.length })}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="destructive"
            disabled={del.isPending}
            onClick={() =>
              del.mutate(
                {
                  name: record.name,
                  type: record.type,
                  set_identifier: record.set_identifier,
                },
                {
                  onSuccess: onClose,
                  onError: (err) =>
                    toast.add({
                      title: t('record.failedToast'),
                      description: err.message,
                      type: 'error',
                    }),
                },
              )
            }
          >
            {t('records.confirmDelete')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
