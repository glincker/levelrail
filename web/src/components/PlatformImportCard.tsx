import { useState, type FormEvent } from 'react'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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

const platforms: { id: ImportPlatform; label: string; tokenLabel: string }[] = [
  { id: 'coolify', label: 'Coolify', tokenLabel: 'API token' },
  { id: 'dokploy', label: 'Dokploy', tokenLabel: 'API key' },
  { id: 'caprover', label: 'CapRover', tokenLabel: 'Login password' },
]

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : 'Request failed'
}

// Three steps: connect (source URL and credential), review (the dry-run
// report with a checkbox per app and database), done (what was created).
// The source token lives only in this component's state, is sent in the
// request body of each step, and is cleared as soon as the import is
// applied or the form is reset. Nothing is cached or persisted.
export function PlatformImportCard() {
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

  const tokenLabel = platforms.find((p) => p.id === platform)?.tokenLabel

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
          <CardTitle>Import summary</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">
            {report.counts.created ?? 0} created, {failed} failed,{' '}
            {report.counts['already-imported'] ?? 0} already imported.
            {failed > 0
              ? ' Run the import again to retry what failed, finished items are skipped.'
              : ''}
          </p>
          {report.notes?.map((n) => (
            <Alert key={n}>
              <WarningIcon />
              <AlertTitle>Data is not migrated</AlertTitle>
              <AlertDescription>{n}</AlertDescription>
            </Alert>
          ))}
          <PlatformImportReportList items={report.items} />
          <Button variant="outline" onClick={reset}>
            Start another import
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
          <CardTitle>Review what will be imported</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {report.items.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              Nothing was found on the source. Check the URL and that the
              credential can see your projects.
            </p>
          ) : (
            <PlatformImportReportList
              items={report.items}
              selected={selected}
              onToggle={toggle}
            />
          )}
          {report.notes?.map((n) => (
            <Alert key={n}>
              <WarningIcon />
              <AlertTitle>Data is not migrated</AlertTitle>
              <AlertDescription>{n}</AlertDescription>
            </Alert>
          ))}
          {apply.isError ? (
            <Alert variant="destructive">
              <AlertTitle>Import failed</AlertTitle>
              <AlertDescription>{errorMessage(apply.error)}</AlertDescription>
            </Alert>
          ) : null}
          {applying ? <Progress value={null} aria-label="Importing" /> : null}
          <div className="flex gap-2">
            <Button
              onClick={onApply}
              disabled={applying || selected.size === 0}
            >
              {applying ? 'Importing...' : `Import ${selected.size} selected`}
            </Button>
            <Button variant="outline" onClick={reset} disabled={applying}>
              Back
            </Button>
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Import from another platform</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={onDiscover} className="space-y-4">
          <p className="text-sm text-muted-foreground">
            Reads apps, databases and settings from the other platform without
            changing it. The credential is sent to this control plane once per
            step and is never stored.
          </p>
          <div className="space-y-1.5">
            <Label>Platform</Label>
            <div className="flex gap-2" role="group" aria-label="Platform">
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
            <Label htmlFor="import-url">Source URL</Label>
            <Input
              id="import-url"
              type="url"
              required
              placeholder="https://coolify.example.com"
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
                aria-label="Skip names already in use"
                checked={skipTaken}
                onCheckedChange={(v) => setSkipTaken(v === true)}
              />
              Skip resources whose name is already taken (default renames them)
            </Label>
            <Label className="flex items-center gap-2 font-normal">
              <Checkbox
                aria-label="Source is on a private network"
                checked={allowPrivate}
                onCheckedChange={(v) => setAllowPrivate(v === true)}
              />
              Source is on a private network (needs the control plane opt-in)
            </Label>
            <Label className="flex items-center gap-2 font-normal">
              <Checkbox
                aria-label="Skip TLS verification"
                checked={insecure}
                onCheckedChange={(v) => setInsecure(v === true)}
              />
              Skip TLS verification (self-signed source)
            </Label>
          </div>
          {discover.isError ? (
            <Alert variant="destructive">
              <AlertTitle>Could not read the source</AlertTitle>
              <AlertDescription>
                {errorMessage(discover.error)}
              </AlertDescription>
            </Alert>
          ) : null}
          <Button type="submit" disabled={discover.isPending}>
            {discover.isPending ? 'Reading source...' : 'Discover'}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
