import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TrashIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import {
  useCreateDnsHealthCheck,
  useDeleteDnsHealthCheck,
  useDnsHealthChecks,
} from '../../queries/dns'
import type { DnsHealthCheck } from '../../types/dns'

const TYPES: DnsHealthCheck['type'][] = ['HTTPS', 'HTTP', 'TCP']

/** HealthChecksPanel manages the Route53 health checks failover and multivalue sets reference. */
export function HealthChecksPanel() {
  const { t } = useTranslation('dns')
  const list = useDnsHealthChecks(true)
  const create = useCreateDnsHealthCheck()
  const del = useDeleteDnsHealthCheck()
  const [type, setType] = useState<DnsHealthCheck['type']>('HTTPS')
  const [target, setTarget] = useState('')
  const [port, setPort] = useState('')
  const [path, setPath] = useState('/')

  function submit() {
    const isIP = /^[\d.]+$|:/.test(target)
    create.mutate(
      {
        type,
        fqdn: isIP ? undefined : target.trim(),
        ip_address: isIP ? target.trim() : undefined,
        port: port ? Number(port) : undefined,
        resource_path: type === 'TCP' ? undefined : path,
      },
      {
        onSuccess: () => setTarget(''),
        onError: (err) =>
          toast.add({
            title: t('health.failedToast'),
            description: err.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">{t('health.lead')}</p>
      <div className="flex flex-wrap items-center gap-2">
        <Select
          value={type}
          onValueChange={(v) => setType(v as DnsHealthCheck['type'])}
        >
          <SelectTrigger aria-label={t('health.type')} className="w-28">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {TYPES.map((x) => (
              <SelectItem key={x} value={x}>
                {x}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Input
          aria-label={t('health.target')}
          placeholder={t('health.target')}
          className="max-w-xs font-mono"
          value={target}
          onChange={(e) => setTarget(e.target.value)}
        />
        <Input
          aria-label={t('health.port')}
          placeholder={t('health.port')}
          inputMode="numeric"
          className="w-24"
          value={port}
          onChange={(e) => setPort(e.target.value)}
        />
        {type === 'TCP' ? null : (
          <Input
            aria-label={t('health.path')}
            className="w-32 font-mono"
            value={path}
            onChange={(e) => setPath(e.target.value)}
          />
        )}
        <Button
          disabled={target.trim() === '' || create.isPending}
          onClick={submit}
        >
          {t('health.create')}
        </Button>
      </div>
      {list.error ? (
        <p className="text-sm text-destructive">{list.error.message}</p>
      ) : null}
      <ul className="divide-y divide-border rounded-md border border-border">
        {(list.data ?? []).map((h) => (
          <li key={h.id} className="flex items-center gap-3 px-3 py-2 text-sm">
            <span className="font-mono text-xs text-muted-foreground">
              {h.id}
            </span>
            <span>{h.type}</span>
            <span className="font-mono">
              {h.fqdn ?? h.ip_address}
              {h.port ? `:${h.port}` : ''}
              {h.resource_path ?? ''}
            </span>
            <Button
              size="icon-sm"
              variant="ghost"
              className="ml-auto"
              aria-label={t('health.delete', { id: h.id })}
              onClick={() => h.id && del.mutate(h.id)}
            >
              <TrashIcon />
            </Button>
          </li>
        ))}
        {list.data && list.data.length === 0 ? (
          <li className="px-3 py-2 text-sm text-muted-foreground">
            {t('health.empty')}
          </li>
        ) : null}
      </ul>
    </div>
  )
}
