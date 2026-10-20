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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  composeValue,
  parseValue,
  RECORD_TYPES,
  ROUTING_POLICIES,
  validateRecord,
  type ValueParts,
} from '../../lib/dnsRecords'
import { useDnsHealthChecks, useSaveDnsRecord } from '../../queries/dns'
import type {
  DnsCapabilities,
  DnsIssue,
  DnsRecordSet,
  DnsRecordType,
  DnsRouting,
} from '../../types/dns'
import { RecordValueRows } from './RecordValueRows'

function initialRows(r?: DnsRecordSet): ValueParts[] {
  if (!r || r.values.length === 0) return [{}]
  return r.values.map((v) => parseValue(r.type as DnsRecordType, v))
}

/** RecordDialog adds or edits one record set with per type fields, validated before the request. */
export function RecordDialog({
  zone,
  zoneName,
  caps,
  record,
  onClose,
}: {
  zone: string
  zoneName: string
  caps: DnsCapabilities
  record?: DnsRecordSet
  onClose: () => void
}) {
  const { t } = useTranslation('dns')
  const save = useSaveDnsRecord(zone)
  const health = useDnsHealthChecks(caps.health_checks)
  const [name, setName] = useState(record?.name ?? '')
  const [type, setType] = useState<DnsRecordType>(
    (record?.type as DnsRecordType) ?? 'A',
  )
  const [rows, setRows] = useState<ValueParts[]>(initialRows(record))
  const [ttl, setTtl] = useState(
    String(record?.ttl ?? (caps.proxied ? 1 : 300)),
  )
  const [proxied, setProxied] = useState(record?.proxied ?? false)
  const [routing, setRouting] = useState<DnsRouting>(
    (record?.routing as DnsRouting) || 'simple',
  )
  const [setId, setSetId] = useState(record?.set_identifier ?? '')
  const [weight, setWeight] = useState(String(record?.weight ?? ''))
  const [failover, setFailover] = useState(record?.failover ?? '')
  const [healthId, setHealthId] = useState(record?.health_check_id ?? '')
  const [issues, setIssues] = useState<DnsIssue[]>([])
  const [touched, setTouched] = useState(false)

  const draft: DnsRecordSet = {
    name: name.trim() || '@',
    type,
    ttl: Number(ttl) || 0,
    values: rows.map((r) => composeValue(type, r)),
    proxied: caps.proxied && proxied,
    routing,
    set_identifier: routing === 'simple' ? undefined : setId.trim(),
    weight:
      routing === 'weighted' && weight !== '' ? Number(weight) : undefined,
    failover: routing === 'failover' ? failover : undefined,
    health_check_id: healthId || undefined,
  }
  const problems = validateRecord(draft, caps)
  const proxiable = caps.proxied && ['A', 'AAAA', 'CNAME'].includes(type)

  function submit() {
    setTouched(true)
    if (problems.length > 0) return
    save.mutate(
      {
        record: draft,
        original: record
          ? {
              name: record.name,
              type: record.type,
              set_identifier: record.set_identifier,
            }
          : undefined,
      },
      {
        onSuccess: (res) => {
          for (const w of res.issues ?? []) {
            toast.add({
              title: t('record.warning'),
              description: w.message,
              type: 'warning',
            })
          }
          toast.add({
            title: t('record.savedToast', { name: res.record.name }),
            type: 'success',
          })
          onClose()
        },
        onError: (err) => {
          setIssues(err.issues)
          if (err.issues.length === 0) {
            toast.add({
              title: t('record.failedToast'),
              description: err.message,
              type: 'error',
            })
          }
        },
      },
    )
  }

  return (
    <Dialog open onOpenChange={(o) => (o ? undefined : onClose())}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {record ? t('record.editTitle') : t('record.addTitle')}
          </DialogTitle>
          <DialogDescription>
            {t('record.lead', { zone: zoneName })}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-[1fr_8rem_6rem]">
            <div className="space-y-1.5">
              <Label htmlFor="dns-record-name">{t('record.name')}</Label>
              <Input
                id="dns-record-name"
                value={name}
                placeholder="@"
                className="font-mono"
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="dns-record-type">{t('record.type')}</Label>
              <Select
                value={type}
                onValueChange={(v) => {
                  setType(v as DnsRecordType)
                  setRows([v === 'CAA' ? { flags: '0', tag: 'issue' } : {}])
                }}
              >
                <SelectTrigger
                  id="dns-record-type"
                  className="w-full"
                  disabled={!!record}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {RECORD_TYPES.map((rt) => (
                    <SelectItem key={rt} value={rt}>
                      {rt}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="dns-record-ttl">{t('record.ttl')}</Label>
              <Input
                id="dns-record-ttl"
                inputMode="numeric"
                value={ttl}
                onChange={(e) => setTtl(e.target.value)}
              />
            </div>
          </div>
          <p className="text-xs text-muted-foreground">
            {t('record.nameHint', { zone: zoneName })}{' '}
            {caps.proxied ? t('record.ttlAutoHint') : null}
          </p>
          <div className="space-y-1.5">
            <Label>{t('record.values')}</Label>
            <RecordValueRows type={type} rows={rows} onChange={setRows} />
          </div>
          {proxiable ? (
            <div className="flex items-center gap-2">
              <Switch
                id="dns-record-proxied"
                checked={proxied}
                onCheckedChange={setProxied}
              />
              <Label htmlFor="dns-record-proxied">{t('record.proxied')}</Label>
            </div>
          ) : null}
          {caps.routing ? (
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="dns-record-routing">
                  {t('record.routing')}
                </Label>
                <Select
                  value={routing}
                  onValueChange={(v) => setRouting(v as DnsRouting)}
                >
                  <SelectTrigger id="dns-record-routing" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {ROUTING_POLICIES.map((p) => (
                      <SelectItem key={p} value={p}>
                        {t(`record.routingPolicy.${p}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              {routing !== 'simple' ? (
                <div className="space-y-1.5">
                  <Label htmlFor="dns-record-setid">
                    {t('record.setIdentifier')}
                  </Label>
                  <Input
                    id="dns-record-setid"
                    value={setId}
                    onChange={(e) => setSetId(e.target.value)}
                  />
                </div>
              ) : null}
              {routing === 'weighted' ? (
                <div className="space-y-1.5">
                  <Label htmlFor="dns-record-weight">
                    {t('record.weight')}
                  </Label>
                  <Input
                    id="dns-record-weight"
                    inputMode="numeric"
                    value={weight}
                    onChange={(e) => setWeight(e.target.value)}
                  />
                </div>
              ) : null}
              {routing === 'failover' ? (
                <div className="space-y-1.5">
                  <Label htmlFor="dns-record-failover">
                    {t('record.failover')}
                  </Label>
                  <Select
                    value={failover}
                    onValueChange={(v) =>
                      setFailover(String(v) as 'PRIMARY' | 'SECONDARY')
                    }
                  >
                    <SelectTrigger id="dns-record-failover" className="w-full">
                      <SelectValue placeholder={t('record.failoverPick')} />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="PRIMARY">
                        {t('record.primary')}
                      </SelectItem>
                      <SelectItem value="SECONDARY">
                        {t('record.secondary')}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              ) : null}
              {routing !== 'simple' && caps.health_checks ? (
                <div className="space-y-1.5">
                  <Label htmlFor="dns-record-health">
                    {t('record.healthCheck')}
                  </Label>
                  <Select
                    value={healthId}
                    onValueChange={(v) => setHealthId(String(v ?? ''))}
                  >
                    <SelectTrigger id="dns-record-health" className="w-full">
                      <SelectValue placeholder={t('record.noHealthCheck')} />
                    </SelectTrigger>
                    <SelectContent>
                      {(health.data ?? []).map((h) => (
                        <SelectItem key={h.id} value={h.id ?? ''}>
                          {h.type} {h.fqdn ?? h.ip_address}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              ) : null}
            </div>
          ) : null}
          {touched && problems.length > 0 ? (
            <ul className="space-y-0.5 text-sm text-destructive" role="alert">
              {problems.map((p) => (
                <li key={p}>{t(`validation.${p}`)}</li>
              ))}
            </ul>
          ) : null}
          {issues.length > 0 ? (
            <ul className="space-y-0.5 text-sm text-destructive" role="alert">
              {issues.map((is) => (
                <li key={is.code}>{is.message}</li>
              ))}
            </ul>
          ) : null}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button disabled={save.isPending} onClick={submit}>
            {save.isPending ? t('record.saving') : t('record.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
