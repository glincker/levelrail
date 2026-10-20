import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { recordId } from '../../lib/dnsRecords'
import { useDnsDiscover, useImportDnsRecords } from '../../queries/dns'

/** ImportExisting resolves common names at the domain's current name servers and imports the ones the operator keeps. */
export function ImportExisting({
  zone,
  onDone,
}: {
  zone: string
  onDone: () => void
}) {
  const { t } = useTranslation('dns')
  const [extra, setExtra] = useState('')
  const [names, setNames] = useState('')
  const discover = useDnsDiscover(zone, names, true)
  const importer = useImportDnsRecords(zone)
  const [skipped, setSkipped] = useState<Set<string>>(new Set())

  const records = useMemo(() => discover.data?.records ?? [], [discover.data])
  const chosen = records.filter((r) => !skipped.has(recordId(r)))

  function toggle(id: string, keep: boolean) {
    setSkipped((prev) => {
      const next = new Set(prev)
      if (keep) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function apply() {
    importer.mutate(
      { format: 'json', records: chosen, apply: true },
      {
        onSuccess: (res) => {
          toast.add({
            title: t('import.appliedToast', { count: res.applied }),
            type: 'success',
          })
          onDone()
        },
        onError: (err) =>
          toast.add({
            title: t('import.failedToast'),
            description: err.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        {t('import.discoverHint')}
      </p>
      <div className="flex items-end gap-2">
        <div className="flex-1 space-y-1.5">
          <Label htmlFor="dns-discover-extra">{t('import.extraNames')}</Label>
          <Input
            id="dns-discover-extra"
            value={extra}
            placeholder={t('import.extraPlaceholder')}
            onChange={(e) => setExtra(e.target.value)}
          />
        </div>
        <Button variant="outline" onClick={() => setNames(extra)}>
          {t('import.rescan')}
        </Button>
      </div>
      {discover.isPending ? (
        <Skeleton className="h-32 w-full" />
      ) : discover.error ? (
        <p className="text-sm text-destructive">{discover.error.message}</p>
      ) : records.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('import.nothingFound')}
        </p>
      ) : (
        <>
          <p className="text-xs text-muted-foreground">
            {t('import.foundAt', {
              count: records.length,
              servers: discover.data?.servers.join(', ') ?? '',
            })}
          </p>
          <ul className="max-h-64 space-y-1 overflow-auto rounded-md border border-border p-2">
            {records.map((r) => {
              const id = recordId(r)
              return (
                <li key={id} className="flex items-start gap-2 text-sm">
                  <Checkbox
                    aria-label={t('import.keep', {
                      name: r.name,
                      type: r.type,
                    })}
                    checked={!skipped.has(id)}
                    onCheckedChange={(v) => toggle(id, v === true)}
                  />
                  <span className="w-28 shrink-0 font-mono">{r.name}</span>
                  <span className="w-14 shrink-0 font-mono">{r.type}</span>
                  <span className="min-w-0 font-mono text-xs break-all text-muted-foreground">
                    {r.values.join(' | ')}
                  </span>
                </li>
              )
            })}
          </ul>
        </>
      )}
      <div className="flex justify-end gap-2">
        <Button variant="outline" onClick={onDone}>
          {t('import.skip')}
        </Button>
        <Button
          disabled={chosen.length === 0 || importer.isPending}
          onClick={apply}
        >
          {t('import.importSelected', { count: chosen.length })}
        </Button>
      </div>
    </div>
  )
}
