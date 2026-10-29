import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ListGlassIcon,
  LogOutGlassIcon,
  ShieldCheckGlassIcon,
  ShieldGlassIcon,
  ShieldOffGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useBrand } from '../hooks/useBrand'
import {
  type AuthGateProvider,
  type DomainAuthGate,
  useClearDomainAuthGate,
  useDomainAuthGate,
  useRevokeAuthGateSessions,
  useSetDomainAuthGate,
} from '../queries/domainAuthGate'
import { type AllowDraft, AuthGateAllowEditor } from './AuthGateAllowEditor'
import { AuthGateAccessLog } from './AuthGateAccessLog'
import { EnforcedBadge, EnforcementNote } from './domains/EnforcementGate'

interface GateForm {
  enabled: boolean
  provider: AuthGateProvider
  authURL: string
  checkPath: string
  copyHeaders: string
  timeout: string
  allow: AllowDraft[]
}

const PROVIDERS: AuthGateProvider[] = ['external', 'builtin']

function formFrom(gate: DomainAuthGate | undefined): GateForm {
  return {
    enabled: gate?.configured ? gate.enabled : true,
    provider: gate?.provider ?? 'external',
    authURL: gate?.auth_url ?? '',
    checkPath: gate?.check_path ?? '/',
    copyHeaders: (gate?.copy_headers ?? []).join(', '),
    timeout: gate?.timeout_seconds ? String(gate.timeout_seconds) : '',
    allow: (gate?.allow ?? []).map((r) => ({ ...r })),
  }
}

// Forward auth for one saved domain: the ingress asks the operator's auth
// service, or this installation's own sign-in, about every request.
export function DomainAuthGateControl({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { t } = useTranslation('domains')
  const brand = useBrand()
  const { data: gate, isLoading } = useDomainAuthGate(appName, domain)
  const [open, setOpen] = useState(false)
  const [showLog, setShowLog] = useState(false)
  const [draft, setDraft] = useState<GateForm | null>(null)
  const [formError, setFormError] = useState<string | null>(null)
  const setGate = useSetDomainAuthGate(appName, domain)
  const clearGate = useClearDomainAuthGate(appName, domain)
  const revoke = useRevokeAuthGateSessions(appName, domain)

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

  const form = draft ?? formFrom(gate)
  const update = (patch: Partial<GateForm>) => setDraft({ ...form, ...patch })
  const on = gate?.enabled ?? false
  const builtin = form.provider === 'builtin'
  const savedBuiltin = gate?.configured && gate.provider === 'builtin'
  const builtinBlocked = builtin && !gate?.builtin_available
  const pending = setGate.isPending || clearGate.isPending
  const id = (field: string) => `auth-gate-${field}-${domain}`

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setFormError(null)
    if (!builtin && !form.authURL.trim()) {
      setFormError(t('authGate.errors.urlRequired'))
      return
    }
    const timeout = form.timeout.trim() === '' ? 0 : Number(form.timeout)
    if (!Number.isInteger(timeout) || timeout < 0) {
      setFormError(t('authGate.errors.timeout'))
      return
    }
    setGate.mutate(
      {
        enabled: form.enabled,
        provider: form.provider,
        auth_url: builtin ? '' : form.authURL.trim(),
        check_path: builtin ? '/' : form.checkPath.trim(),
        copy_headers: form.copyHeaders
          .split(',')
          .map((h) => h.trim())
          .filter((h) => h.length > 0),
        timeout_seconds: timeout,
        allow: builtin
          ? form.allow.map((r) => ({ kind: r.kind, subject: r.subject }))
          : [],
      },
      { onSuccess: () => setDraft(null) },
    )
  }

  return (
    <div className="rounded-md border border-border bg-muted/30 p-3 text-sm">
      <div className="flex items-center justify-between gap-2">
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          className="flex items-center gap-1.5 text-left"
        >
          {on ? (
            <EnforcedBadge domain={domain} control="auth_gate">
              <Badge variant="success" className="shrink-0">
                <ShieldCheckGlassIcon className="size-3.5 text-tone-success" />
                {t('authGate.badgeOn')}
              </Badge>
            </EnforcedBadge>
          ) : (
            <Badge variant="muted" className="shrink-0">
              <ShieldOffGlassIcon className="size-3.5" />
              {t('authGate.badgeOff')}
            </Badge>
          )}
        </button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => setOpen((v) => !v)}
        >
          <ShieldGlassIcon className="size-4" />
          {open
            ? t('authGate.hide')
            : on
              ? t('authGate.manage')
              : t('authGate.setUp')}
        </Button>
      </div>
      <EnforcementNote domain={domain} control="auth_gate" active={on} />

      {open ? (
        <form onSubmit={handleSubmit} className="mt-3 space-y-3">
          <p className="text-muted-foreground">{t('authGate.description')}</p>
          <Field orientation="horizontal">
            <FieldLabel htmlFor={id('enabled')}>
              {t('authGate.enabled')}
            </FieldLabel>
            <Switch
              id={id('enabled')}
              checked={form.enabled}
              onCheckedChange={(checked) => update({ enabled: checked })}
              disabled={pending}
            />
          </Field>
          <Field>
            <FieldLabel>{t('authGate.providerLabel')}</FieldLabel>
            <div
              role="radiogroup"
              aria-label={t('authGate.providerLabel')}
              className="flex flex-wrap gap-1"
            >
              {PROVIDERS.map((p) => (
                <Button
                  key={p}
                  type="button"
                  size="sm"
                  role="radio"
                  aria-checked={form.provider === p}
                  variant={form.provider === p ? 'secondary' : 'ghost'}
                  onClick={() => update({ provider: p })}
                  disabled={pending}
                >
                  {p === 'builtin'
                    ? t('authGate.providerBuiltin', { name: brand.Name })
                    : t('authGate.providerExternal')}
                </Button>
              ))}
            </div>
            {builtin ? (
              <FieldDescription>
                {t('authGate.builtinHelp', { name: brand.Name })}
              </FieldDescription>
            ) : null}
          </Field>

          {builtinBlocked ? (
            <Alert variant="destructive">
              <AlertDescription>
                {t('authGate.builtinUnavailable')}
              </AlertDescription>
            </Alert>
          ) : null}

          {builtin ? (
            <AuthGateAllowEditor
              id={id('allow')}
              rules={form.allow}
              onChange={(allow) => update({ allow })}
              disabled={pending}
            />
          ) : (
            <>
              <Field>
                <FieldLabel htmlFor={id('url')}>
                  {t('authGate.urlLabel')}
                </FieldLabel>
                <Input
                  id={id('url')}
                  autoComplete="off"
                  value={form.authURL}
                  onChange={(e) => update({ authURL: e.target.value })}
                  disabled={pending}
                  placeholder="http://auth:9091"
                />
                <FieldDescription>{t('authGate.urlHelp')}</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor={id('path')}>
                  {t('authGate.checkPathLabel')}
                </FieldLabel>
                <Input
                  id={id('path')}
                  autoComplete="off"
                  value={form.checkPath}
                  onChange={(e) => update({ checkPath: e.target.value })}
                  disabled={pending}
                  placeholder="/check"
                />
              </Field>
            </>
          )}
          <Field>
            <FieldLabel htmlFor={id('headers')}>
              {builtin
                ? t('authGate.extraHeadersLabel')
                : t('authGate.copyHeadersLabel')}
            </FieldLabel>
            <Input
              id={id('headers')}
              autoComplete="off"
              value={form.copyHeaders}
              onChange={(e) => update({ copyHeaders: e.target.value })}
              disabled={pending}
              placeholder={builtin ? '' : 'X-User, X-Email, X-Name'}
            />
            <FieldDescription>{t('authGate.copyHeadersHelp')}</FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor={id('timeout')}>
              {t('authGate.timeoutLabel')}
            </FieldLabel>
            <Input
              id={id('timeout')}
              inputMode="numeric"
              value={form.timeout}
              onChange={(e) => update({ timeout: e.target.value })}
              disabled={pending}
              placeholder={String(gate?.default_timeout_seconds ?? '')}
            />
            <FieldDescription>
              {t('authGate.timeoutHelp', {
                seconds: gate?.default_timeout_seconds ?? 0,
              })}
            </FieldDescription>
          </Field>

          {formError ? (
            <Alert variant="destructive">
              <AlertDescription>{formError}</AlertDescription>
            </Alert>
          ) : null}
          {setGate.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{setGate.error.message}</AlertDescription>
            </Alert>
          ) : null}
          {clearGate.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{clearGate.error.message}</AlertDescription>
            </Alert>
          ) : null}
          {revoke.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{revoke.error.message}</AlertDescription>
            </Alert>
          ) : null}

          <div className="flex flex-wrap items-center gap-2">
            <Button
              type="submit"
              size="sm"
              disabled={pending || builtinBlocked}
            >
              {setGate.isPending ? t('authGate.saving') : t('authGate.save')}
            </Button>
            {gate?.configured ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={pending}
                onClick={() =>
                  clearGate.mutate(undefined, {
                    onSuccess: () => setDraft(null),
                  })
                }
              >
                {clearGate.isPending
                  ? t('authGate.removing')
                  : t('authGate.remove')}
              </Button>
            ) : null}
          </div>

          {savedBuiltin ? (
            <div className="space-y-3 border-t border-border pt-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="text-muted-foreground">
                  {t('authGate.liveSessions', {
                    count: gate.live_sessions,
                  })}
                </span>
                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={revoke.isPending || gate.live_sessions === 0}
                    onClick={() =>
                      revoke.mutate(undefined, {
                        onSuccess: (res) =>
                          toast.add({
                            title: t('authGate.revoked', {
                              count: res.revoked,
                            }),
                            type: 'success',
                          }),
                      })
                    }
                  >
                    <LogOutGlassIcon className="size-4" />
                    {revoke.isPending
                      ? t('authGate.revoking')
                      : t('authGate.revokeSessions')}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    onClick={() => setShowLog((v) => !v)}
                  >
                    <ListGlassIcon className="size-4" />
                    {showLog
                      ? t('authGate.accessLog.hide')
                      : t('authGate.accessLog.show')}
                  </Button>
                </div>
              </div>
              {showLog ? (
                <AuthGateAccessLog appName={appName} domain={domain} />
              ) : null}
            </div>
          ) : null}
        </form>
      ) : null}
    </div>
  )
}
