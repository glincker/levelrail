import { useMemo, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  CheckIcon,
  GlobeIcon,
  PencilSimpleIcon,
  PlusIcon,
  TrashIcon,
  WarningIcon,
  WarningCircleIcon,
  XIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { VariantProps } from 'class-variance-authority'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ApiError } from '../lib/apiError'
import {
  useCreateDnsRecord,
  useDeleteDnsRecord,
  useDnsRecords,
  useUpdateDnsRecord,
  type DnsRecord,
} from '../queries/dnsRecords'

const RECORD_TYPES = ['A', 'AAAA', 'CNAME', 'TXT', 'MX', 'SRV', 'CAA']

const STATUS_META: Record<
  string,
  { label: string; variant: VariantProps<typeof badgeVariants>['variant'] }
> = {
  resolved: { label: 'Resolved', variant: 'success' },
  pending: { label: 'Pending', variant: 'warning' },
  mismatch: { label: 'Mismatch', variant: 'destructive' },
  unknown: { label: 'Unverified', variant: 'muted' },
}

// A record's identity for edit/delete purposes: libdns's own exact-match
// contract (name + type + value together) is the only thing this view
// can key on, since DNS records have no provider-wide ID.
function recordKey(r: DnsRecord): string {
  return `${r.type}:${r.name}:${r.value}`
}

type DraftRecord = { name: string; type: string; value: string; ttl: string }

const EMPTY_DRAFT: DraftRecord = { name: '', type: 'A', value: '', ttl: '300' }

function draftFromRecord(r: DnsRecord): DraftRecord {
  return {
    name: r.name,
    type: r.type,
    value: r.value,
    ttl: String(r.ttl_seconds),
  }
}

function toRecord(d: DraftRecord): DnsRecord {
  return {
    name: d.name.trim(),
    type: d.type,
    value: d.value.trim(),
    ttl_seconds: Math.max(0, Number.parseInt(d.ttl, 10) || 0),
  }
}

// Shared row of inputs for both the "add" form and an in-place row edit,
// so the two never drift apart on which fields exist.
function RecordFields({
  draft,
  onChange,
  idPrefix,
}: {
  draft: DraftRecord
  onChange: (next: DraftRecord) => void
  idPrefix: string
}) {
  return (
    <div className="grid grid-cols-[6rem_1fr_1fr_5rem] gap-2">
      <Select
        value={draft.type}
        onValueChange={(value) =>
          onChange({ ...draft, type: value ?? draft.type })
        }
      >
        <SelectTrigger id={`${idPrefix}-type`} aria-label="Record type">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {RECORD_TYPES.map((t) => (
            <SelectItem key={t} value={t}>
              {t}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Input
        aria-label="Record name"
        className="font-mono"
        placeholder="@ or subdomain"
        value={draft.name}
        onChange={(e) => onChange({ ...draft, name: e.target.value })}
      />
      <Input
        aria-label="Record value"
        className="font-mono"
        placeholder="Value"
        value={draft.value}
        onChange={(e) => onChange({ ...draft, value: e.target.value })}
      />
      <Input
        aria-label="TTL seconds"
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

function RecordRow({
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
              <CheckIcon aria-hidden="true" />
              <span className="sr-only">Save</span>
            </Button>
            <Button
              type="button"
              size="icon-sm"
              variant="ghost"
              disabled={saving}
              onClick={onCancelEdit}
            >
              <XIcon aria-hidden="true" />
              <span className="sr-only">Cancel</span>
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
            <Badge variant={status.variant}>{status.label}</Badge>
          ) : null}
          <Button
            type="button"
            size="icon-sm"
            variant="ghost"
            onClick={onStartEdit}
          >
            <PencilSimpleIcon aria-hidden="true" />
            <span className="sr-only">Edit record</span>
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
              <TrashIcon aria-hidden="true" />
              <span className="sr-only">Delete record</span>
            </DialogTrigger>
            <DialogContent className="sm:max-w-sm">
              <DialogHeader>
                <DialogTitle className="flex items-center gap-1.5 text-destructive">
                  <WarningIcon className="size-4" aria-hidden="true" />
                  Delete this record?
                </DialogTitle>
                <DialogDescription>
                  {record.type} {record.name || '@'} pointing to {record.value}{' '}
                  will be removed from the zone. This cannot be undone.
                </DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setConfirmingDelete(false)}
                >
                  Cancel
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
                  {deleting ? 'Deleting...' : 'Delete'}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>
      </TableCell>
    </TableRow>
  )
}

// A per-domain DNS records view: lists the whole best-effort zone's
// A/AAAA/CNAME/TXT/MX/SRV/CAA records (not just this one domain's own
// host) from whichever ACME DNS-01 provider is already configured, with
// add/edit/delete and a live resolved/pending/mismatch check per record.
// Collapsed by default, the same pattern DomainWafControl establishes
// for another per-domain panel that shouldn't dominate DomainEditor's
// list when a domain has nothing unusual configured.
export function DomainDnsRecordsControl({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const [open, setOpen] = useState(false)
  const { data, isLoading, isError, error } = useDnsRecords(appName, domain)
  const createRecord = useCreateDnsRecord(appName, domain)
  const updateRecord = useUpdateDnsRecord(appName, domain)
  const deleteRecord = useDeleteDnsRecord(appName, domain)
  const [editingKey, setEditingKey] = useState<string | null>(null)
  const [addDraft, setAddDraft] = useState<DraftRecord>(EMPTY_DRAFT)
  const [adding, setAdding] = useState(false)

  const records = data?.records ?? []
  const notConfigured =
    isError && error instanceof ApiError && error.status === 501

  const parentRef = useMemo(
    () => ({ current: null as HTMLDivElement | null }),
    [],
  )
  const virtualizer = useVirtualizer({
    count: records.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 44,
    overscan: 10,
  })

  if (!open) {
    return (
      <div className="flex items-center justify-between gap-2 rounded-md border border-border bg-muted/30 p-3 text-sm">
        <button
          type="button"
          onClick={() => setOpen(true)}
          aria-expanded={false}
          className="flex items-center gap-1.5 text-left"
        >
          <Badge variant="muted" className="shrink-0">
            <GlobeIcon className="size-3" aria-hidden="true" />
            {isLoading
              ? 'DNS records'
              : notConfigured
                ? 'DNS records: no provider'
                : `${records.length} DNS record${records.length === 1 ? '' : 's'}`}
          </Badge>
        </button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          aria-expanded={false}
          onClick={() => setOpen(true)}
        >
          <GlobeIcon className="size-3.5" aria-hidden="true" />
          Manage DNS records
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-3 rounded-md border border-border bg-muted/30 p-3 text-sm">
      <div className="flex items-center justify-between gap-2">
        <span className="flex items-center gap-1.5 font-medium text-foreground">
          <GlobeIcon className="size-4" aria-hidden="true" />
          DNS records{data?.zone ? ` for ${data.zone.replace(/\.$/, '')}` : ''}
        </span>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          aria-expanded={true}
          onClick={() => setOpen(false)}
        >
          Hide
        </Button>
      </div>

      {isLoading ? (
        <Skeleton className="h-24 w-full" />
      ) : notConfigured ? (
        <Alert>
          <WarningCircleIcon className="size-4" aria-hidden="true" />
          <AlertDescription>
            No DNS provider is configured for records management. Enable
            Cloudflare DNS or Route53 DNS above to list and edit this zone's
            records here.
          </AlertDescription>
        </Alert>
      ) : isError ? (
        <Alert variant="destructive">
          <AlertDescription>{error?.message}</AlertDescription>
        </Alert>
      ) : (
        <>
          {records.length === 0 ? (
            <p className="text-xs text-muted-foreground">No records found.</p>
          ) : (
            <div
              ref={(el) => {
                parentRef.current = el
              }}
              className="max-h-80 overflow-auto rounded-md border border-border"
            >
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Type</TableHead>
                    <TableHead>Name</TableHead>
                    <TableHead>Value</TableHead>
                    <TableHead>TTL</TableHead>
                    <TableHead className="text-right">Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody
                  role="rowgroup"
                  style={{
                    display: 'grid',
                    height: virtualizer.getTotalSize(),
                  }}
                >
                  {virtualizer.getVirtualItems().map((row) => {
                    const record = records[row.index]
                    if (!record) {
                      return null
                    }
                    const key = recordKey(record)
                    return (
                      <div
                        key={key}
                        role="presentation"
                        style={{
                          display: 'grid',
                          position: 'absolute',
                          top: 0,
                          left: 0,
                          width: '100%',
                          transform: `translateY(${row.start}px)`,
                        }}
                      >
                        <RecordRow
                          record={record}
                          editing={editingKey === key}
                          saving={updateRecord.isPending}
                          deleting={deleteRecord.isPending}
                          onStartEdit={() => setEditingKey(key)}
                          onCancelEdit={() => setEditingKey(null)}
                          onSave={(draft) => {
                            updateRecord.mutate(
                              { original: record, record: toRecord(draft) },
                              { onSuccess: () => setEditingKey(null) },
                            )
                          }}
                          onDelete={() => {
                            deleteRecord.mutate(record)
                          }}
                        />
                      </div>
                    )
                  })}
                </TableBody>
              </Table>
            </div>
          )}

          <div className="space-y-2 border-t border-border pt-3">
            <FieldLabel className="text-xs">Add a record</FieldLabel>
            <Field orientation="horizontal" className="items-start">
              <RecordFields
                draft={addDraft}
                onChange={setAddDraft}
                idPrefix="add"
              />
              <Button
                type="button"
                size="icon-sm"
                disabled={
                  adding ||
                  createRecord.isPending ||
                  addDraft.value.trim().length === 0
                }
                onClick={() => {
                  setAdding(true)
                  createRecord.mutate(toRecord(addDraft), {
                    onSuccess: () => {
                      setAddDraft(EMPTY_DRAFT)
                    },
                    onSettled: () => setAdding(false),
                  })
                }}
              >
                <PlusIcon aria-hidden="true" />
                <span className="sr-only">Add record</span>
              </Button>
            </Field>
            {createRecord.isError ? (
              <Alert variant="destructive">
                <AlertDescription>
                  {createRecord.error.message}
                </AlertDescription>
              </Alert>
            ) : null}
            {updateRecord.isError ? (
              <Alert variant="destructive">
                <AlertDescription>
                  {updateRecord.error.message}
                </AlertDescription>
              </Alert>
            ) : null}
            {deleteRecord.isError ? (
              <Alert variant="destructive">
                <AlertDescription>
                  {deleteRecord.error.message}
                </AlertDescription>
              </Alert>
            ) : null}
          </div>
        </>
      )}
    </div>
  )
}
