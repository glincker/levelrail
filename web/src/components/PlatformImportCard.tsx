import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import {
  useApplyPlatformImport,
  useDiscoverPlatformImport,
  type ImportPlatform,
  type PlatformImportReport,
  type PlatformImportRequest,
} from '../queries/platformImport'
import { PlatformImportReportList } from './PlatformImportReportList'
import { isSelectable, itemKey } from '../lib/platformImport'

type Step = 'connect' | 'review' | 'done'

const platforms: {
  id: ImportPlatform
  label: string
  tokenKey: 'tokenApi' | 'tokenKey' | 'tokenPassword'
}[] = [
  { id: 'coolify', label: 'Coolify', tokenKey: 'tokenApi' },
  { id: 'dokploy', label: 'Dokploy', tokenKey: 'tokenKey' },
  { id: 'caprover', label: 'CapRover', tokenKey: 'tokenPassword' },
  { id: 'heroku', label: 'Heroku', tokenKey: 'tokenKey' },
]

// Three steps: connect (source URL and credential), review (the dry-run
// report with a checkbox per app and database), done (what was created).
// The source token lives only in this component's state, is sent in the
// request body of each step, and is cleared as soon as the import is
// applied or the form is reset. Nothing is cached or persisted.
export function PlatformImportCard() {
  const { t } = useTranslation('migration')
  const errorMessage = (err: unknown): string =>
    err instanceof Error ? err.message : t('platformImport.requestFailed')
  const [step, setStep] = useState<Step>('connect')
  const [platform, setPlatform] = useState<ImportPlatform>('coolify')
  const [url, setUrl] = useState('')
  const [token, setToken] = useState('')
  const [insecure, setInsecure] = useState(false)
  const [allowPrivate, setAllowPrivate] = useState(false)
  const [skipTaken, setSkipTaken] = useState(false)
  const [report, setReport] = useState<PlatformImportReport | null>(null)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const discover = useDiscoverPlatformImport()
  const apply = useApplyPlatformImport()

  const tokenKey = platforms.find((p) => p.id === platform)?.tokenKey
  const tokenLabel = tokenKey ? t(`platformImport.${tokenKey}`) : undefined

  function baseRequest(): PlatformImportRequest {
    return {
      platform,
      url: url.trim(),
      token,
      insecure_tls: insecure || undefined,
      allow_private: allowPrivate || undefined,
      collision: skipTaken ? 'skip' : 'suffix',
    }
  }

  function reset() {
    setToken('')
    setReport(null)
    setSelected(new Set())
    discover.reset()
    apply.reset()
    setStep('connect')
  }

  function onDiscover(e: FormEvent) {
    e.preventDefault()
    discover.mutate(baseRequest(), {
      onSuccess: (r) => {
        setReport(r)
        setSelected(new Set(r.items.filter(isSelectable).map(itemKey)))
        setStep('review')
      },
    })
  }

  function onApply() {
    if (!report) return
    const only = report.items
      .filter((i) => selected.has(itemKey(i)))
      .map((i) => i.source_id)
    apply.mutate(
      { ...baseRequest(), only },
      {
        onSuccess: (r) => {
          setReport(r)
          setToken('')
          setStep('done')
        },
      },
    )
  }

  function toggle(key: string, next: boolean) {
    setSelected((prev) => {
      const copy = new Set(prev)
      if (next) copy.add(key)
      else copy.delete(key)
      return copy
    })
  }

  if (step === 'done' && report) {
    const failed = report.counts.failed ?? 0
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('platformImport.summaryTitle')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">
            {t('platformImport.summaryCounts', {
              created: report.counts.created ?? 0,
              failed,
              already: report.counts['already-imported'] ?? 0,
            })}
            {failed > 0 ? ` ${t('platformImport.retryHint')}` : ''}
          </p>
          {report.notes?.map((n) => (
            <Banner
              key={n}
              variant="warning"
              title={t('platformImport.dataNotMigrated')}
            >
              {n}
            </Banner>
          ))}
          <PlatformImportReportList items={report.items} />
          <Button variant="outline" onClick={reset}>
            {t('platformImport.another')}
          </Button>
        </CardContent>
      </Card>
    )
  }

  if (step === 'review' && report) {
    const applying = apply.isPending
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('platformImport.reviewTitle')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {report.items.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {t('platformImport.nothingFound')}
            </p>
          ) : (
            <PlatformImportReportList
              items={report.items}
              selected={selected}
              onToggle={toggle}
            />
          )}
          {report.notes?.map((n) => (
            <Banner
              key={n}
              variant="warning"
              title={t('platformImport.dataNotMigrated')}
            >
              {n}
            </Banner>
          ))}
          {apply.isError ? (
            <Banner variant="critical" title={t('platformImport.importFailed')}>
              {errorMessage(apply.error)}
            </Banner>
          ) : null}
          {applying ? (
            <Progress value={null} aria-label={t('platformImport.importing')} />
          ) : null}
          <div className="flex gap-2">
            <Button
              onClick={onApply}
              disabled={applying || selected.size === 0}
            >
              {applying
                ? t('platformImport.importing')
                : t('platformImport.importSelected', { count: selected.size })}
            </Button>
            <Button variant="outline" onClick={reset} disabled={applying}>
              {t('platformImport.back')}
            </Button>
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('platformImport.connectTitle')}</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={onDiscover} className="space-y-4">
          <p className="text-sm text-muted-foreground">
            {t('platformImport.connectIntro')}
          </p>
          <div className="space-y-1.5">
            <Label>{t('platformImport.platform')}</Label>
            <div
              className="flex gap-2"
              role="group"
              aria-label={t('platformImport.platform')}
            >
              {platforms.map((p) => (
                <Button
                  key={p.id}
                  type="button"
                  variant={platform === p.id ? 'default' : 'outline'}
                  aria-pressed={platform === p.id}
                  onClick={() => setPlatform(p.id)}
                >
                  {p.label}
                </Button>
              ))}
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="import-url">{t('platformImport.sourceUrl')}</Label>
            <Input
              id="import-url"
              type="url"
              required={platform !== 'heroku'}
              placeholder={
                platform === 'heroku'
                  ? t('platformImport.herokuUrlPlaceholder')
                  : 'https://coolify.example.com'
              }
              value={url}
              onChange={(e) => setUrl(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="import-token">{tokenLabel}</Label>
            <Input
              id="import-token"
              type="password"
              autoComplete="off"
              required
              value={token}
              onChange={(e) => setToken(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label className="flex items-center gap-2 font-normal">
              <Checkbox
                aria-label={t('platformImport.skipTakenLabel')}
                checked={skipTaken}
                onCheckedChange={(v) => setSkipTaken(v === true)}
              />
              {t('platformImport.skipTaken')}
            </Label>
            <Label className="flex items-center gap-2 font-normal">
              <Checkbox
                aria-label={t('platformImport.privateLabel')}
                checked={allowPrivate}
                onCheckedChange={(v) => setAllowPrivate(v === true)}
              />
              {t('platformImport.private')}
            </Label>
            <Label className="flex items-center gap-2 font-normal">
              <Checkbox
                aria-label={t('platformImport.insecureLabel')}
                checked={insecure}
                onCheckedChange={(v) => setInsecure(v === true)}
              />
              {t('platformImport.insecure')}
            </Label>
          </div>
          {discover.isError ? (
            <Banner variant="critical" title={t('platformImport.readFailed')}>
              {errorMessage(discover.error)}
            </Banner>
          ) : null}
          <Button type="submit" disabled={discover.isPending}>
            {discover.isPending
              ? t('platformImport.reading')
              : t('platformImport.discover')}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
