import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  PencilGlassIcon,
  TrashGlassIcon,
  TriangleAlertGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import type { VariantProps } from 'class-variance-authority'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { TableCell, TableRow } from '@/components/ui/table'
import type { DnsRecord } from '../queries/dnsRecords'
import {
  draftFromRecord,
  recordKey,
  type DraftRecord,
} from '../lib/dnsRecordDraft'

const RECORD_TYPES = ['A', 'AAAA', 'CNAME', 'TXT', 'MX', 'SRV', 'CAA']

const STATUS_META: Record<
  string,
  {
    key: 'statusResolved' | 'statusPending' | 'statusMismatch' | 'statusUnknown'
    variant: VariantProps<typeof badgeVariants>['variant']
  }
> = {
  resolved: { key: 'statusResolved', variant: 'success' },
  pending: { key: 'statusPending', variant: 'warning' },
  mismatch: { key: 'statusMismatch', variant: 'destructive' },
  unknown: { key: 'statusUnknown', variant: 'muted' },
}

// Shared row of inputs for both the "add" form and an in-place row edit,
// so the two never drift apart on which fields exist.
export function RecordFields({
  draft,
  onChange,
  idPrefix,
}: {
  draft: DraftRecord
  onChange: (next: DraftRecord) => void
  idPrefix: string
}) {
  const { t } = useTranslation('dns')
  return (
    <div className="grid grid-cols-[6rem_1fr_1fr_5rem] gap-2">
      <Select
        value={draft.type}
        onValueChange={(value) =>
          onChange({ ...draft, type: value ?? draft.type })
        }
      >
        <SelectTrigger
          id={`${idPrefix}-type`}
          aria-label={t('recordsControl.recordType')}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {RECORD_TYPES.map((type) => (
            <SelectItem key={type} value={type}>
              {type}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Input
        aria-label={t('recordsControl.recordName')}
        className="font-mono"
        placeholder={t('recordsControl.namePlaceholder')}
        value={draft.name}
        onChange={(e) => onChange({ ...draft, name: e.target.value })}
      />
      <Input
        aria-label={t('recordsControl.recordValue')}
        className="font-mono"
        placeholder={t('recordsControl.valuePlaceholder')}
        value={draft.value}
        onChange={(e) => onChange({ ...draft, value: e.target.value })}
      />
      <Input
        aria-label={t('recordsControl.ttlSeconds')}
        type="number"
        min={0}
        inputMode="numeric"
        placeholder="300"
        value={draft.ttl}
        onChange={(e) => onChange({ ...draft, ttl: e.target.value })}
      />
    </div>
  )
}

export function RecordRow({
  record,
  editing,
  onStartEdit,
  onCancelEdit,
  onSave,
  onDelete,
  saving,
  deleting,
}: {
  record: DnsRecord
  editing: boolean
  onStartEdit: () => void
  onCancelEdit: () => void
  onSave: (next: DraftRecord) => void
  onDelete: () => void
  saving: boolean
  deleting: boolean
}) {
  const { t } = useTranslation('dns')
  const [draft, setDraft] = useState<DraftRecord>(() => draftFromRecord(record))
  const [confirmingDelete, setConfirmingDelete] = useState(false)

  if (editing) {
    return (
      <TableRow>
        <TableCell colSpan={5}>
          <div className="flex items-center gap-2">
            <RecordFields
              draft={draft}
              onChange={setDraft}
              idPrefix={`edit-${recordKey(record)}`}
            />
            <Button
              type="button"
              size="icon-sm"
              disabled={saving}
              onClick={() => onSave(draft)}
            >
              <CheckGlassIcon size="1.4em" aria-hidden="true" />
              <span className="sr-only">{t('recordsControl.save')}</span>
            </Button>
            <Button
              type="button"
              size="icon-sm"
              variant="ghost"
              disabled={saving}
              onClick={onCancelEdit}
            >
              <XGlassIcon size="1.4em" aria-hidden="true" />
              <span className="sr-only">{t('recordsControl.cancel')}</span>
            </Button>
          </div>
        </TableCell>
      </TableRow>
    )
  }

  const status = record.status ? STATUS_META[record.status] : undefined

  return (
    <TableRow>
      <TableCell className="font-mono text-xs">{record.type}</TableCell>
      <TableCell className="max-w-[10rem] truncate font-mono text-xs">
        {record.name || '@'}
      </TableCell>
      <TableCell className="max-w-[16rem] truncate font-mono text-xs">
        {record.value}
      </TableCell>
      <TableCell className="text-xs text-muted-foreground">
        {record.ttl_seconds}s
      </TableCell>
      <TableCell>
        <div className="flex items-center justify-end gap-1">
          {status ? (
            <Badge variant={status.variant}>
              {t(`recordsControl.${status.key}`)}
            </Badge>
          ) : null}
          <Button
            type="button"
            size="icon-sm"
            variant="ghost"
            onClick={onStartEdit}
          >
            <PencilGlassIcon size="1.4em" aria-hidden="true" />
            <span className="sr-only">{t('recordsControl.editRecord')}</span>
          </Button>
          <Dialog open={confirmingDelete} onOpenChange={setConfirmingDelete}>
            <DialogTrigger
              render={
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  disabled={deleting}
                />
              }
            >
              <TrashGlassIcon size="1.4em" aria-hidden="true" />
              <span className="sr-only">
                {t('recordsControl.deleteRecord')}
              </span>
            </DialogTrigger>
            <DialogContent className="sm:max-w-sm">
              <DialogHeader>
                <DialogTitle className="flex items-center gap-1.5 text-destructive">
                  <TriangleAlertGlassIcon
                    className="size-6 text-tone-warning"
                    aria-hidden="true"
                  />
                  {t('recordsControl.deleteTitle')}
                </DialogTitle>
                <DialogDescription>
                  {t('recordsControl.deleteBody', {
                    type: record.type,
                    name: record.name || '@',
                    value: record.value,
                  })}
                </DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setConfirmingDelete(false)}
                >
                  {t('recordsControl.cancel')}
                </Button>
                <Button
                  type="button"
                  variant="destructive"
                  disabled={deleting}
                  onClick={() => {
                    setConfirmingDelete(false)
                    onDelete()
                  }}
                >
                  {deleting
                    ? t('recordsControl.deleting')
                    : t('recordsControl.delete')}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>
      </TableCell>
    </TableRow>
  )
}
