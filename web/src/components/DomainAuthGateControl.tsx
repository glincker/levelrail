import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ShieldCheckIcon,
  ShieldIcon,
  ShieldSlashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import {
  type DomainAuthGate,
  useClearDomainAuthGate,
  useDomainAuthGate,
  useSetDomainAuthGate,
} from '../queries/domainAuthGate'

interface GateForm {
  enabled: boolean
  authURL: string
  checkPath: string
  copyHeaders: string
  timeout: string
}

function formFrom(gate: DomainAuthGate | undefined): GateForm {
  return {
    enabled: gate?.configured ? gate.enabled : true,
    authURL: gate?.auth_url ?? '',
    checkPath: gate?.check_path ?? '/',
    copyHeaders: (gate?.copy_headers ?? []).join(', '),
    timeout: gate?.timeout_seconds ? String(gate.timeout_seconds) : '',
  }
}

// Forward auth for one saved domain: the ingress asks the operator's auth
// service about every request before it reaches the app.
export function DomainAuthGateControl({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { t } = useTranslation('domains')
  const { data: gate, isLoading } = useDomainAuthGate(appName, domain)
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState<GateForm | null>(null)
  const [formError, setFormError] = useState<string | null>(null)
  const setGate = useSetDomainAuthGate(appName, domain)
  const clearGate = useClearDomainAuthGate(appName, domain)

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
  const pending = setGate.isPending || clearGate.isPending
  const id = (field: string) => `auth-gate-${field}-${domain}`

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setFormError(null)
    if (!form.authURL.trim()) {
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
        auth_url: form.authURL.trim(),
        check_path: form.checkPath.trim(),
        copy_headers: form.copyHeaders
          .split(',')
          .map((h) => h.trim())
          .filter((h) => h.length > 0),
        timeout_seconds: timeout,
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
            <Badge variant="success" className="shrink-0">
              <ShieldCheckIcon className="size-3" />
              {t('authGate.badgeOn')}
            </Badge>
          ) : (
            <Badge variant="muted" className="shrink-0">
              <ShieldSlashIcon className="size-3" />
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
          <ShieldIcon className="size-3.5" />
          {open
            ? t('authGate.hide')
            : on
              ? t('authGate.manage')
              : t('authGate.setUp')}
        </Button>
      </div>

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
          <Field>
            <FieldLabel htmlFor={id('headers')}>
              {t('authGate.copyHeadersLabel')}
            </FieldLabel>
            <Input
              id={id('headers')}
              autoComplete="off"
              value={form.copyHeaders}
              onChange={(e) => update({ copyHeaders: e.target.value })}
              disabled={pending}
              placeholder="X-User, X-Email, X-Name"
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

          <div className="flex items-center gap-2">
            <Button type="submit" size="sm" disabled={pending}>
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
        </form>
      ) : null}
    </div>
  )
}
