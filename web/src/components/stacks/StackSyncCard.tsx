import { useEffect, useId, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CopyGlassIcon,
  RefreshCwGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
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
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { IacCodeBadge } from '../iac/IacCodeBadge'
import { ApiError } from '../../lib/apiError'
import { formatRelativeTime } from '../../lib/relativeTime'
import {
  stackSyncQueryOptions,
  usePutStackSync,
  useRefreshStackSync,
  useRotateStackSyncSecret,
  type StackSyncConfig,
  type StackSyncInfo,
  type StackSyncSecret,
  type StackSyncStatus,
} from '../../queries/stackSync'

const KNOWN_CODES = new Set([
  'sync_repo_unreachable',
  'sync_path_invalid',
  'sync_limits_exceeded',
  'sync_plan_failed',
  'sync_secret_in_repo',
  'sync_disk_low',
  'sync_apply_failed',
])

// Mirrors the server refresh cooldown default; the server message is still shown.
const COOLDOWN_MS =
  (Number(import.meta.env.VITE_SYNC_REFRESH_COOLDOWN_SECS) || 30) * 1000

const COPIED_MS = 1500
const EMPTY: StackSyncConfig = {
  repo_url: '',
  branch: 'main',
  path: '',
  credential_ref: '',
  interval_seconds: undefined,
  webhook_only: false,
  enabled: false,
  apply: false,
  prune: false,
  deploy: false,
}

function SwitchRow({
  label,
  hint,
  warn,
  checked,
  disabled,
  onChange,
}: Readonly<{
  label: string
  hint: string
  warn?: boolean
  checked: boolean
  disabled: boolean
  onChange: (next: boolean) => void
}>) {
  return (
    <div
      className={
        warn
          ? 'flex items-center justify-between gap-3 rounded-lg border border-destructive/40 bg-destructive/5 px-2 py-1.5'
          : 'flex items-center justify-between gap-3 px-2 py-1.5'
      }
    >
      <div className="min-w-0">
        <div className="text-sm font-medium">{label}</div>
        <div className="text-xs text-muted-foreground">{hint}</div>
      </div>
      <Switch
        checked={checked}
        disabled={disabled}
        aria-label={label}
        onCheckedChange={onChange}
      />
    </div>
  )
}

function Field({
  label,
  value,
  onChange,
  disabled,
  placeholder,
  type,
}: Readonly<{
  label: string
  value: string
  onChange: (v: string) => void
  disabled: boolean
  placeholder?: string
  type?: string
}>) {
  const id = useId()
  return (
    <div className="min-w-0 space-y-1">
      <Label htmlFor={id} className="text-xs">
        {label}
      </Label>
      <Input
        id={id}
        type={type}
        value={value}
        placeholder={placeholder}
        disabled={disabled}
        onChange={(e) => {
          onChange(e.target.value)
        }}
      />
    </div>
  )
}

function StatusRow({
  info,
  onViewPlan,
}: Readonly<{ info: StackSyncInfo; onViewPlan?: (rev: number) => void }>) {
  const { t } = useTranslation('stackSync')
  const s: StackSyncStatus | undefined = info.status
  const enabled = info.config?.enabled === true
  const counts = s?.counts ?? {}
  const code = s?.code
  return (
    <div className="space-y-1.5">
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <Badge variant={enabled ? 'success' : 'secondary'}>
          {enabled ? t('status.enabled') : t('status.disabled')}
        </Badge>
        {s ? (
          <>
            {s.commit ? (
              <span className="font-mono">
                {t('status.commit', { sha: s.commit.slice(0, 7) })}
              </span>
            ) : null}
            {s.checked_at ? (
              <span className="text-muted-foreground">
                {t('status.checked', {
                  when: formatRelativeTime(s.checked_at),
                })}
              </span>
            ) : null}
            {s.applied_commit ? (
              <span className="font-mono text-muted-foreground">
                {t('status.applied', { sha: s.applied_commit.slice(0, 7) })}
              </span>
            ) : null}
            {s.pending ? (
              <>
                <Badge variant="warning">{t('status.pending')}</Badge>
                {(['create', 'update', 'delete'] as const).map((k) => (
                  <span key={k}>
                    {t(`status.${k}`, { count: counts[k] ?? 0 })}
                  </span>
                ))}
                {s.revision && onViewPlan ? (
                  <Button
                    variant="link"
                    size="xs"
                    onClick={() => {
                      onViewPlan(s.revision ?? 0)
                    }}
                  >
                    {t('status.viewPlan')}
                  </Button>
                ) : null}
              </>
            ) : code ? null : (
              <span className="text-muted-foreground">
                {t('status.inSync')}
              </span>
            )}
          </>
        ) : (
          <span className="text-muted-foreground">
            {t('status.neverSynced')}
          </span>
        )}
      </div>
      {code ? (
        <div className="flex flex-wrap items-center gap-2 text-xs">
          <IacCodeBadge code={code} />
          <span>
            {KNOWN_CODES.has(code) ? t(`codes.${code}`) : t('codes.unknown')}
          </span>
          {s?.detail ? (
            <span className="font-mono break-all text-muted-foreground">
              {s.detail}
            </span>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

function RefreshButton({
  name,
  disabled,
}: Readonly<{ name: string; disabled: boolean }>) {
  const { t } = useTranslation('stackSync')
  const refresh = useRefreshStackSync(name)
  const [cooling, setCooling] = useState<string | null>(null)

  useEffect(() => {
    if (cooling === null) return
    const id = setTimeout(() => {
      setCooling(null)
    }, COOLDOWN_MS)
    return () => {
      clearTimeout(id)
    }
  }, [cooling])

  function run() {
    refresh.mutate(undefined, {
      onError: (e) => {
        if (e instanceof ApiError && e.status === 429) {
          setCooling(e.message)
          return
        }
        toast.add({
          title: t('refresh.failed'),
          description: e.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <div className="flex items-center gap-2">
      <Button
        variant="outline"
        size="sm"
        disabled={disabled || refresh.isPending || cooling !== null}
        title={t('refresh.hint')}
        onClick={run}
      >
        <RefreshCwGlassIcon aria-hidden="true" />
        {refresh.isPending ? t('refresh.running') : t('refresh.action')}
      </Button>
      {cooling ? (
        <span role="status" className="text-xs text-muted-foreground">
          {t('refresh.cooldown', { message: cooling })}
        </span>
      ) : null}
    </div>
  )
}

function CopyRow({ label, value }: Readonly<{ label: string; value: string }>) {
  const { t } = useTranslation('stackSync')
  const id = useId()
  const [copied, setCopied] = useState(false)
  return (
    <div className="space-y-1">
      <Label htmlFor={id} className="text-xs">
        {label}
      </Label>
      <div className="flex items-center gap-2">
        <Input
          id={id}
          readOnly
          value={value}
          className="min-w-0 font-mono text-xs"
        />
        <Button
          variant="outline"
          size="icon-sm"
          aria-label={t('webhook.copy', { what: label })}
          onClick={() => {
            void navigator.clipboard.writeText(value).then(() => {
              setCopied(true)
              window.setTimeout(() => {
                setCopied(false)
              }, COPIED_MS)
            })
          }}
        >
          {copied ? (
            <CheckGlassIcon aria-hidden="true" />
          ) : (
            <CopyGlassIcon aria-hidden="true" />
          )}
        </Button>
        <span role="status" className="sr-only">
          {copied ? t('webhook.copied') : ''}
        </span>
      </div>
    </div>
  )
}

function WebhookSection({
  name,
  secretSet,
  disabled,
}: Readonly<{ name: string; secretSet: boolean; disabled: boolean }>) {
  const { t } = useTranslation('stackSync')
  const rotate = useRotateStackSyncSecret(name)
  const [confirming, setConfirming] = useState(false)
  const [revealed, setRevealed] = useState<StackSyncSecret | null>(null)
  const rotateRef = useRef<HTMLButtonElement>(null)

  function closeReveal() {
    setRevealed(null)
    rotate.reset()
  }

  return (
    <div className="flex flex-wrap items-center justify-between gap-2">
      <div className="flex items-center gap-2 text-sm">
        <span className="font-medium">{t('webhook.title')}</span>
        <Badge variant={secretSet ? 'success' : 'secondary'}>
          {secretSet ? t('webhook.secretSet') : t('webhook.noSecret')}
        </Badge>
      </div>
      <Button
        ref={rotateRef}
        variant="outline"
        size="sm"
        disabled={disabled}
        onClick={() => {
          setConfirming(true)
        }}
      >
        {t('webhook.rotate')}
      </Button>
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('webhook.confirmTitle')}
        description={t('webhook.confirmBody')}
        confirmLabel={t('webhook.confirm')}
        pendingLabel={t('webhook.rotating')}
        pending={rotate.isPending}
        error={rotate.isError ? rotate.error.message : null}
        onConfirm={() => {
          rotate.mutate(undefined, {
            onSuccess: (res) => {
              setConfirming(false)
              setRevealed(res)
            },
          })
        }}
      />
      <Dialog
        open={revealed !== null}
        onOpenChange={(open) => {
          if (!open) closeReveal()
        }}
      >
        {/* The confirm dialog that opened this one is gone, so focus returns to the trigger by ref. */}
        <DialogContent finalFocus={rotateRef}>
          <DialogHeader>
            <DialogTitle>{t('webhook.newTitle')}</DialogTitle>
            <DialogDescription className="flex items-center gap-1.5 text-destructive">
              <TriangleAlertGlassIcon aria-hidden="true" className="size-4" />
              {t('webhook.once')}
            </DialogDescription>
          </DialogHeader>
          {revealed ? (
            <div className="space-y-2">
              <CopyRow label={t('webhook.secret')} value={revealed.secret} />
              <CopyRow label={t('webhook.syncUrl')} value={revealed.sync_url} />
              <CopyRow
                label={t('webhook.refreshUrl')}
                value={revealed.refresh_url}
              />
            </div>
          ) : null}
          <DialogFooter>
            <Button onClick={closeReveal}>{t('webhook.done')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function SyncForm({
  name,
  initial,
  disabled,
}: Readonly<{ name: string; initial: StackSyncConfig; disabled: boolean }>) {
  const { t } = useTranslation('stackSync')
  const put = usePutStackSync(name)
  const [cfg, setCfg] = useState<StackSyncConfig>(initial)
  const set = <K extends keyof StackSyncConfig>(
    k: K,
    v: StackSyncConfig[K],
  ) => {
    setCfg((c) => ({ ...c, [k]: v }))
  }
  const off = disabled || put.isPending

  function save() {
    put.mutate(
      {
        ...cfg,
        credential_ref: cfg.credential_ref?.trim() || undefined,
        apply: cfg.apply,
        prune: cfg.apply && cfg.prune,
        deploy: cfg.apply && cfg.deploy,
      },
      {
        onSuccess: () => {
          toast.add({ title: t('form.saved'), type: 'success' })
        },
        onError: (e) => {
          toast.add({
            title: t('form.saveFailed'),
            description: e.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <form
      className="space-y-2"
      onSubmit={(e) => {
        e.preventDefault()
        save()
      }}
    >
      <div className="grid gap-2 sm:grid-cols-4">
        <div className="sm:col-span-2">
          <Field
            label={t('form.repoUrl')}
            value={cfg.repo_url}
            disabled={off}
            onChange={(v) => {
              set('repo_url', v)
            }}
          />
        </div>
        <Field
          label={t('form.branch')}
          value={cfg.branch}
          disabled={off}
          onChange={(v) => {
            set('branch', v)
          }}
        />
        <Field
          label={t('form.path')}
          value={cfg.path}
          disabled={off}
          onChange={(v) => {
            set('path', v)
          }}
        />
      </div>
      <div className="grid gap-2 sm:grid-cols-4">
        <div className="sm:col-span-2">
          <Field
            label={t('form.credential')}
            value={cfg.credential_ref ?? ''}
            placeholder={t('form.credentialPlaceholder')}
            disabled={off}
            onChange={(v) => {
              set('credential_ref', v)
            }}
          />
        </div>
        <Field
          label={t('form.interval')}
          type="number"
          value={cfg.interval_seconds ? String(cfg.interval_seconds) : ''}
          disabled={off || cfg.webhook_only}
          onChange={(v) => {
            set('interval_seconds', v === '' ? undefined : Number(v))
          }}
        />
      </div>
      <p className="text-xs text-muted-foreground">
        {t('form.credentialHint')}
      </p>
      <SwitchRow
        label={t('form.webhookOnly')}
        hint={t('form.webhookOnlyHint')}
        checked={cfg.webhook_only}
        disabled={off}
        onChange={(v) => {
          set('webhook_only', v)
        }}
      />
      <SwitchRow
        label={t('form.apply')}
        hint={t('form.applyHint')}
        checked={cfg.apply}
        disabled={off}
        onChange={(v) => {
          setCfg((c) => ({
            ...c,
            apply: v,
            prune: v && c.prune,
            deploy: v && c.deploy,
          }))
        }}
      />
      <SwitchRow
        warn
        label={t('form.prune')}
        hint={cfg.apply ? t('form.pruneHint') : t('form.pruneNeedsApply')}
        checked={cfg.apply && cfg.prune}
        disabled={off || !cfg.apply}
        onChange={(v) => {
          set('prune', v)
        }}
      />
      <SwitchRow
        warn
        label={t('form.deploy')}
        hint={cfg.apply ? t('form.deployHint') : t('form.deployNeedsApply')}
        checked={cfg.apply && cfg.deploy}
        disabled={off || !cfg.apply}
        onChange={(v) => {
          set('deploy', v)
        }}
      />
      <SwitchRow
        label={t('form.enabled')}
        hint={t('form.enabledHint')}
        checked={cfg.enabled}
        disabled={off}
        onChange={(v) => {
          set('enabled', v)
        }}
      />
      <div className="flex items-center justify-between gap-2">
        <p className="text-xs text-muted-foreground">{t('readOnlyNote')}</p>
        <Button type="submit" size="sm" disabled={off}>
          {put.isPending ? t('form.saving') : t('form.save')}
        </Button>
      </div>
    </form>
  )
}

// Hidden when the control plane has no sync support (the GET fails).
export function StackSyncCard({
  name,
  isRoot,
  requireApproval,
  onViewPlan,
}: Readonly<{
  name: string
  isRoot: boolean
  requireApproval: boolean
  onViewPlan?: (rev: number) => void
}>) {
  const { t } = useTranslation('stackSync')
  const query = useQuery(stackSyncQueryOptions(name))
  const info = query.data
  if (!info) return null
  const initial = info.config ?? EMPTY

  return (
    <section
      className="space-y-3 rounded-xl border border-border bg-card p-3 shadow-sm"
      aria-label={t('title')}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-semibold">{t('title')}</h3>
        <RefreshButton
          name={name}
          disabled={!isRoot || !info.config?.enabled}
        />
      </div>
      <StatusRow info={info} onViewPlan={onViewPlan} />
      {requireApproval ? (
        <p className="text-xs text-muted-foreground">
          {t('approval.required')}
        </p>
      ) : null}
      <SyncForm
        key={JSON.stringify(info.config)}
        name={name}
        initial={initial}
        disabled={!isRoot}
      />
      <WebhookSection
        name={name}
        secretSet={info.webhook_secret_set}
        disabled={!isRoot}
      />
      <p className="text-xs text-muted-foreground">{t('secretsRefused')}</p>
      {isRoot ? null : (
        <p className="text-xs text-muted-foreground">{t('rootOnly')}</p>
      )}
    </section>
  )
}
