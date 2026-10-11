import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { fqdn, RECORD_TYPES } from '../../lib/dnsRecords'
import { useDnsCheck } from '../../queries/dns'

/** PropagationChecker asks the system resolver, public resolvers and the zone's own servers for one record. */
export function PropagationChecker({
  zone,
  zoneName,
}: {
  zone: string
  zoneName: string
}) {
  const { t } = useTranslation('dns')
  const [name, setName] = useState('@')
  const [type, setType] = useState('A')
  const [asked, setAsked] = useState('')
  const check = useDnsCheck(asked, type, zone)

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-2">
        <Input
          aria-label={t('check.name')}
          className="max-w-xs font-mono"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <span className="pb-2 font-mono text-sm text-muted-foreground">
          .{zoneName}
        </span>
        <Select value={type} onValueChange={(v) => setType(String(v))}>
          <SelectTrigger aria-label={t('check.type')} className="w-28">
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
        <Button
          disabled={check.isFetching}
          onClick={() => {
            const target = fqdn(name.trim(), zoneName)
            if (target === asked) void check.refetch()
            else setAsked(target)
          }}
        >
          {check.isFetching ? t('check.checking') : t('check.run')}
        </Button>
      </div>
      {check.error ? (
        <p className="text-sm text-destructive">{check.error.message}</p>
      ) : null}
      {check.data ? (
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <Badge variant={check.data.agree ? 'success' : 'warning'}>
              {check.data.agree ? t('check.agree') : t('check.differ')}
            </Badge>
            {check.data.expected ? (
              <span className="font-mono text-xs text-muted-foreground">
                {t('check.zoneSays', {
                  values: check.data.expected.join(' | '),
                })}
              </span>
            ) : null}
          </div>
          <table className="w-full text-sm">
            <thead className="text-left text-xs text-muted-foreground uppercase">
              <tr>
                <th className="py-1 font-medium">{t('check.col.server')}</th>
                <th className="py-1 font-medium">{t('check.col.ttl')}</th>
                <th className="py-1 font-medium">{t('check.col.answer')}</th>
              </tr>
            </thead>
            <tbody>
              {check.data.answers.map((a) => (
                <tr
                  key={`${a.source}-${a.server}`}
                  className="border-t border-border"
                >
                  <td className="py-1.5 font-mono">
                    {a.server === 'system' ? t('delegation.system') : a.server}
                    {a.source === 'authoritative' ? (
                      <Badge variant="outline" className="ml-1">
                        {t('check.authoritative')}
                      </Badge>
                    ) : null}
                  </td>
                  <td className="py-1.5">
                    {a.error ? '-' : t('check.ttlLeft', { ttl: a.ttl })}
                  </td>
                  <td className="py-1.5 font-mono text-xs break-all">
                    {a.error ?? (a.values?.join(' | ') || t('check.noRecords'))}
                    {a.matches === false ? (
                      <Badge variant="warning" className="ml-1">
                        {t('check.mismatch')}
                      </Badge>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </div>
  )
}
