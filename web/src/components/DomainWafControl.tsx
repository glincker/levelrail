import { useState } from 'react'
import { ShieldCheckIcon, ShieldIcon, ShieldWarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import {
  useClearDomainWaf,
  useDomainWaf,
  useSetDomainWaf,
  type SetDomainWafRequest,
} from '../queries/domainWaf'

// Opt-in Web Application Firewall (OWASP Coraza) and rate limiting for
// one already-saved domain, enforced by the embedded Caddy ingress on
// the next reconcile pass (internal/reconcile/ingress). Collapsed by
// default the same way DomainBasicAuthControl is: a status badge plus a
// toggle, expanding into a form for the two independent settings.
export function DomainWafControl({ appName, domain }: { appName: string; domain: string }) {
  const { data: waf, isLoading } = useDomainWaf(appName, domain)
  const [open, setOpen] = useState(false)
  const [wafEnabled, setWafEnabled] = useState<boolean | null>(null)
  const [wafMode, setWafMode] = useState<'detect' | 'block' | null>(null)
  const [rps, setRps] = useState<string | null>(null)
  const [burst, setBurst] = useState<string | null>(null)
  const setWaf = useSetDomainWaf(appName, domain)
  const clearWaf = useClearDomainWaf(appName, domain)

  if (isLoading) {
    return (
      <div
        className="flex items-center justify-between gap-2 rounded-md border border-border bg-muted/30 p-3"
        aria-hidden="true"
      >
        <Skeleton className="h-5 w-28 rounded-full" />
        <Skeleton className="h-7 w-32" />
      </div>
    )
  }

  const active = (waf?.waf_enabled ?? false) || (waf?.rate_limit_enabled ?? false)
  const effectiveWafEnabled = wafEnabled ?? waf?.waf_enabled ?? false
  const effectiveMode = wafMode ?? waf?.waf_mode ?? 'detect'
  const effectiveRps = rps ?? String(waf?.rate_limit_rps ?? 0)
  const effectiveBurst = burst ?? String(waf?.rate_limit_burst ?? 0)
  const pending = setWaf.isPending || clearWaf.isPending

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const req: SetDomainWafRequest = {
      waf_enabled: effectiveWafEnabled,
      waf_mode: effectiveMode,
      rate_limit_rps: Math.max(0, Number.parseInt(effectiveRps, 10) || 0),
      rate_limit_burst: Math.max(0, Number.parseInt(effectiveBurst, 10) || 0),
    }
    setWaf.mutate(req)
  }

  return (
    <div className="rounded-md border border-border bg-muted/30 p-3 text-sm">
      <div className="flex items-center justify-between gap-2">
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          className="flex items-center gap-1.5 text-left"
        >
          {active ? (
            <Badge variant="success" className="shrink-0">
              <ShieldCheckIcon className="size-3" />
              WAF/rate limit on
            </Badge>
          ) : (
            <Badge variant="muted" className="shrink-0">
              <ShieldIcon className="size-3" />
              Not protected
            </Badge>
          )}
        </button>
        <Button type="button" variant="ghost" size="sm" onClick={() => setOpen((v) => !v)}>
          <ShieldIcon className="size-3.5" />
          {open ? 'Hide' : active ? 'Manage' : 'Add WAF / rate limit'}
        </Button>
      </div>

      {open ? (
        <form
          onSubmit={(e) => {
            handleSubmit(e)
          }}
          className="mt-3 space-y-3"
        >
          <Field orientation="horizontal">
            <FieldLabel htmlFor={`waf-enabled-${domain}`}>OWASP Coraza WAF</FieldLabel>
            <Switch
              id={`waf-enabled-${domain}`}
              checked={effectiveWafEnabled}
              onCheckedChange={(checked) => setWafEnabled(checked)}
              disabled={pending}
            />
          </Field>

          {effectiveWafEnabled ? (
            <Field>
              <FieldLabel htmlFor={`waf-mode-${domain}`}>Mode</FieldLabel>
              <Select
                value={effectiveMode}
                onValueChange={(value) => setWafMode(value)}
                disabled={pending}
              >
                <SelectTrigger id={`waf-mode-${domain}`}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="detect">Detect (log only, recommended to start)</SelectItem>
                  <SelectItem value="block">Block (reject matching requests)</SelectItem>
                </SelectContent>
              </Select>
              <FieldDescription>
                {effectiveMode === 'detect' ? (
                  <span className="flex items-center gap-1">
                    <ShieldWarningIcon className="size-3.5 shrink-0" />
                    Requests matching OWASP CRS rules are logged, never rejected. Switch to Block
                    once you have reviewed logs for false positives.
                  </span>
                ) : (
                  'Requests matching OWASP CRS rules are rejected with an error.'
                )}
              </FieldDescription>
            </Field>
          ) : null}

          <Field>
            <FieldLabel htmlFor={`waf-rps-${domain}`}>Rate limit (requests/sec per IP)</FieldLabel>
            <Input
              id={`waf-rps-${domain}`}
              type="number"
              min={0}
              inputMode="numeric"
              value={effectiveRps}
              onChange={(e) => setRps(e.target.value)}
              disabled={pending}
              placeholder="0 (disabled)"
            />
            <FieldDescription>0 disables rate limiting for this domain.</FieldDescription>
          </Field>

          {Number.parseInt(effectiveRps, 10) > 0 ? (
            <Field>
              <FieldLabel htmlFor={`waf-burst-${domain}`}>Burst allowance</FieldLabel>
              <Input
                id={`waf-burst-${domain}`}
                type="number"
                min={0}
                inputMode="numeric"
                value={effectiveBurst}
                onChange={(e) => setBurst(e.target.value)}
                disabled={pending}
                placeholder="0 (no extra burst)"
              />
              <FieldDescription>
                Requests allowed in a single second beyond the sustained rate above. Leave at 0 for
                a strict cap.
              </FieldDescription>
            </Field>
          ) : null}

          {setWaf.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{setWaf.error.message}</AlertDescription>
            </Alert>
          ) : null}
          {clearWaf.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{clearWaf.error.message}</AlertDescription>
            </Alert>
          ) : null}

          <div className="flex items-center gap-2">
            <Button type="submit" size="sm" disabled={pending}>
              {setWaf.isPending ? 'Saving...' : 'Save'}
            </Button>
            {active ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={pending}
                onClick={() => {
                  clearWaf.mutate(undefined, {
                    onSuccess: () => {
                      setWafEnabled(null)
                      setWafMode(null)
                      setRps(null)
                      setBurst(null)
                    },
                  })
                }}
              >
                {clearWaf.isPending ? 'Removing...' : 'Turn off'}
              </Button>
            ) : null}
          </div>
        </form>
      ) : null}
    </div>
  )
}
