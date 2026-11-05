import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ShieldAlertGlassIcon,
  ShieldCheckGlassIcon,
  ShieldGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { HelpLink } from '@/components/HelpLink'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import {
  useClearDomainWaf,
  useDomainWaf,
  useSetDomainWaf,
  type SetDomainWafRequest,
} from '../queries/domainWaf'
import { SettingSummary } from './domains/SettingSummary'
import { EnforcedBadge, EnforcementNote } from './domains/EnforcementGate'

// Opt-in Web Application Firewall (OWASP Coraza) and rate limiting for
// one already-saved domain, enforced by the embedded Caddy ingress on
// the next reconcile pass (internal/reconcile/ingress). The summary
// always shows mode and rate numbers; the form opens on an explicit Edit.
export function DomainWafControl({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { t } = useTranslation('domains')
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
        className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border bg-muted/30 p-3"
        aria-hidden="true"
      >
        <Skeleton className="h-5 w-28 rounded-full" />
        <Skeleton className="h-7 w-32" />
      </div>
    )
  }

  const active =
    (waf?.waf_enabled ?? false) || (waf?.rate_limit_enabled ?? false)
  const effectiveWafEnabled = wafEnabled ?? waf?.waf_enabled ?? false
  const effectiveMode = wafMode ?? waf?.waf_mode ?? 'detect'
  const effectiveRps = rps ?? String(waf?.rate_limit_rps ?? 0)
  const effectiveBurst = burst ?? String(waf?.rate_limit_burst ?? 0)
  const wafSummary = waf?.waf_enabled
    ? t('waf.summaryWaf', { mode: waf.waf_mode ?? 'detect' })
    : t('waf.summaryWafOff')
  const rateSummary =
    (waf?.rate_limit_rps ?? 0) > 0
      ? t('waf.summaryRate', {
          rps: waf?.rate_limit_rps ?? 0,
          burst: waf?.rate_limit_burst ?? 0,
        })
      : t('waf.summaryRateOff')
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
    <SettingSummary
      status={
        active ? (
          <EnforcedBadge domain={domain} control="waf">
            <Badge variant="success" className="shrink-0">
              <ShieldCheckGlassIcon className="size-4.5 text-tone-success" />
              {t('waf.badgeOn')}
            </Badge>
          </EnforcedBadge>
        ) : (
          <Badge variant="muted" className="shrink-0">
            <ShieldGlassIcon className="size-4.5" />
            {t('waf.badgeOff')}
          </Badge>
        )
      }
      summary={[wafSummary, rateSummary].join(' · ')}
      note={<EnforcementNote domain={domain} control="waf" active={active} />}
      actions={
        <HelpLink
          path="/domains-and-ingress#opt-in-waf-and-rate-limiting"
          label={t('waf.helpLabel')}
        />
      }
      editLabel={active ? t('waf.edit') : t('waf.add')}
      closeLabel={t('waf.close')}
      editing={open}
      onEditingChange={setOpen}
    >
      <form
        onSubmit={(e) => {
          handleSubmit(e)
        }}
        className="mt-3 space-y-3"
      >
        <Field orientation="horizontal">
          <FieldLabel htmlFor={`waf-enabled-${domain}`}>
            {t('waf.wafLabel')}
          </FieldLabel>
          <Switch
            id={`waf-enabled-${domain}`}
            checked={effectiveWafEnabled}
            onCheckedChange={(checked) => setWafEnabled(checked)}
            disabled={pending}
          />
        </Field>

        {effectiveWafEnabled ? (
          <Field>
            <FieldLabel htmlFor={`waf-mode-${domain}`}>
              {t('waf.modeLabel')}
            </FieldLabel>
            <Select
              value={effectiveMode}
              onValueChange={(value) => setWafMode(value)}
              disabled={pending}
            >
              <SelectTrigger id={`waf-mode-${domain}`}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="detect">{t('waf.modeDetect')}</SelectItem>
                <SelectItem value="block">{t('waf.modeBlock')}</SelectItem>
              </SelectContent>
            </Select>
            <FieldDescription>
              {effectiveMode === 'detect' ? (
                <span className="flex items-center gap-1">
                  <ShieldAlertGlassIcon className="size-5 shrink-0 text-tone-warning" />
                  {t('waf.helpDetect')}
                </span>
              ) : (
                t('waf.helpBlock')
              )}
            </FieldDescription>
          </Field>
        ) : null}

        <Field>
          <FieldLabel htmlFor={`waf-rps-${domain}`}>
            {t('waf.rpsLabel')}
          </FieldLabel>
          <Input
            id={`waf-rps-${domain}`}
            type="number"
            min={0}
            inputMode="numeric"
            value={effectiveRps}
            onChange={(e) => setRps(e.target.value)}
            disabled={pending}
            placeholder={t('waf.rpsPlaceholder')}
          />
          <FieldDescription>{t('waf.rpsHelp')}</FieldDescription>
        </Field>

        {Number.parseInt(effectiveRps, 10) > 0 ? (
          <Field>
            <FieldLabel htmlFor={`waf-burst-${domain}`}>
              {t('waf.burstLabel')}
            </FieldLabel>
            <Input
              id={`waf-burst-${domain}`}
              type="number"
              min={0}
              inputMode="numeric"
              value={effectiveBurst}
              onChange={(e) => setBurst(e.target.value)}
              disabled={pending}
              placeholder={t('waf.burstPlaceholder')}
            />
            <FieldDescription>{t('waf.burstHelp')}</FieldDescription>
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
            {setWaf.isPending ? t('waf.saving') : t('waf.save')}
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
              {clearWaf.isPending ? t('waf.removing') : t('waf.turnOff')}
            </Button>
          ) : null}
        </div>
      </form>
    </SettingSummary>
  )
}
