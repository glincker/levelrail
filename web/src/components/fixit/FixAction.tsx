import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  RefreshCwGlassIcon,
  WrenchGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { useIsRoot } from '../../hooks/useIsRoot'
import { claimAutoApply } from '../../lib/fixAutoApply'
import { FixCli } from './FixCli'
import { formatBytes } from '../../lib/format'
import {
  useApplyFix,
  useFixForCheck,
  type Fix,
  type FixResult,
  type FixRisk,
} from '../../queries/fixit'

const RISK_VARIANT: Record<FixRisk, 'success' | 'warning' | 'destructive'> = {
  safe: 'success',
  restarts: 'warning',
  exposes: 'destructive',
}

/**
 * FixAction anatomy: a primary button (Fix now, or Re-check for a step done
 * elsewhere), a result line, and a collapsed Fix via CLI option. autoApply
 * runs a safe auto fix once per subject per session on first render.
 */
export function FixAction({
  fix,
  autoApply = false,
  onResult,
}: {
  fix: Fix
  autoApply?: boolean
  /** Called after any apply or re-check, so a page can refresh the check it shows. */
  onResult?: (r: FixResult) => void
}) {
  const { t } = useTranslation('fixit')
  const isRoot = useIsRoot()
  const apply = useApplyFix()
  const [confirm, setConfirm] = useState(false)
  const [param, setParam] = useState(fix.param?.default ?? '')
  const [result, setResult] = useState<FixResult | null>(null)

  const auto = fix.kind === 'auto'
  const rateLimited = fix.state === 'rate_limited'
  const params = fix.param ? { [fix.param.name]: param } : undefined

  const autoTried = useRef(false)
  const canAutoApply =
    autoApply &&
    isRoot &&
    auto &&
    fix.risk === 'safe' &&
    fix.reversible &&
    fix.state === 'offer' &&
    !fix.param
  useEffect(() => {
    if (!canAutoApply || autoTried.current) return
    autoTried.current = true
    if (!claimAutoApply(fix.id, fix.subject)) return
    apply.mutate(
      { id: fix.id },
      {
        onSuccess: (r) => {
          setResult(r)
          onResult?.(r)
        },
      },
    )
  }, [canAutoApply, fix.id, fix.subject, apply, onResult])

  function run() {
    apply.mutate(
      { id: fix.id, params },
      {
        onSuccess: (r) => {
          setResult(r)
          setConfirm(false)
          onResult?.(r)
        },
      },
    )
  }

  return (
    <div className="mt-2 space-y-2 rounded-md border border-border p-3">
      <div className="flex flex-wrap items-center gap-2">
        <WrenchGlassIcon className="size-5 shrink-0" aria-hidden="true" />
        <p className="text-sm font-medium text-foreground">{fix.title}</p>
        <Badge variant={RISK_VARIANT[fix.risk]}>{t(`risk.${fix.risk}`)}</Badge>
        <Badge variant="muted">{t(`kind.${fix.kind}`)}</Badge>
        <span className="text-xs text-muted-foreground">
          {fix.reversible ? t('reversible') : t('irreversible')}
        </span>
      </div>
      <p className="text-sm text-muted-foreground">{fix.why}</p>
      {fix.reason ? (
        <p className="text-sm text-muted-foreground">{fix.reason}</p>
      ) : null}

      {fix.changes.length > 0 ? (
        <div>
          <p className="text-xs font-medium text-foreground">
            {t('willChange')}
          </p>
          <ul className="mt-1 list-disc space-y-0.5 pl-5 text-xs text-muted-foreground">
            {fix.changes.map((c) => (
              <li key={c.label + (c.detail ?? '')}>
                {c.label}
                {c.detail ? `: ${c.detail}` : ''}
                {c.bytes ? ` (${formatBytes(c.bytes)})` : ''}
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      {fix.blocked && !rateLimited ? (
        <p className="text-xs text-muted-foreground">{fix.blocked}</p>
      ) : null}
      {rateLimited ? (
        <p className="text-xs text-muted-foreground">{t('rateLimited')}</p>
      ) : null}

      {auto && fix.param ? (
        <div>
          <label
            className="text-xs font-medium text-foreground"
            htmlFor={`fix-${fix.id}`}
          >
            {t('paramLabel')}
          </label>
          <Input
            id={`fix-${fix.id}`}
            list={`fix-${fix.id}-options`}
            value={param}
            placeholder={t('paramPlaceholder')}
            onChange={(e) => {
              setParam(e.target.value)
            }}
          />
          <datalist id={`fix-${fix.id}-options`}>
            {fix.param.options.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </datalist>
        </div>
      ) : null}

      <div className="flex flex-wrap items-center gap-2">
        {auto && isRoot ? (
          <Button
            type="button"
            size="sm"
            disabled={apply.isPending || rateLimited}
            onClick={() => {
              setConfirm(true)
            }}
          >
            {apply.isPending ? t('applying') : t('apply')}
          </Button>
        ) : null}
        {isRoot && (!auto || result) ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={apply.isPending}
            onClick={run}
          >
            <RefreshCwGlassIcon size="1.4em" />
            {apply.isPending ? t('rechecking') : t('recheck')}
          </Button>
        ) : null}
      </div>

      {apply.isError ? (
        <p role="alert" className="text-xs text-destructive">
          {apply.error.message}
        </p>
      ) : null}
      {result ? <FixResultLine result={result} /> : null}
      {fix.command && !auto ? (
        <FixCli steps={[{ command: fix.command }]} />
      ) : null}

      <ConfirmDialog
        open={confirm}
        onOpenChange={setConfirm}
        title={t('confirmTitle')}
        description={t('confirmBody', { title: fix.title })}
        consequences={fix.changes.map(
          (c) => c.label + (c.detail ? `: ${c.detail}` : ''),
        )}
        confirmLabel={t('apply')}
        pendingLabel={t('applying')}
        pending={apply.isPending}
        error={apply.isError ? apply.error.message : null}
        onConfirm={run}
      />
    </div>
  )
}

function FixResultLine({ result }: { result: FixResult }) {
  const { t } = useTranslation('fixit')
  const good = result.outcome === 'fixed' || result.outcome === 'skipped'
  const Icon = good ? CircleCheckGlassIcon : CircleAlertGlassIcon
  return (
    <p
      role="status"
      className={`flex items-start gap-1.5 text-xs ${good ? 'text-foreground' : 'text-destructive'}`}
    >
      <Icon className="mt-px size-3.5 shrink-0" aria-hidden="true" />
      {t(`outcome.${result.outcome}`, { detail: result.detail })}
    </p>
  )
}

/** ManualFix is the anatomy for a check with no typed fix: Re-check first, the command collapsed. */
export function ManualFix({
  message,
  command,
  pending,
  onRecheck,
}: {
  message?: string
  command?: string
  pending: boolean
  onRecheck: () => void
}) {
  const { t } = useTranslation('fixit')
  return (
    <div className="space-y-2">
      {message ? (
        <p className="text-xs text-muted-foreground">{message}</p>
      ) : null}
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={pending}
        onClick={onRecheck}
      >
        <RefreshCwGlassIcon size="1.4em" />
        {pending ? t('rechecking') : t('recheck')}
      </Button>
      {command ? <FixCli steps={[{ command }]} /> : null}
    </div>
  )
}

/** CheckFix renders the engine's fix for a failing check id, or nothing. */
export function CheckFix({ checkId }: { checkId: string }) {
  const fix = useFixForCheck(checkId)
  if (!fix) return null
  return <FixAction fix={fix} autoApply />
}
