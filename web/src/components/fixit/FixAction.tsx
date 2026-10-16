import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowsClockwiseIcon,
  CheckIcon,
  CopyIcon,
  WrenchIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { useIsRoot } from '../../hooks/useIsRoot'
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

function CommandBlock({ command }: { command: string }) {
  const { t } = useTranslation('fixit')
  const [copied, setCopied] = useState(false)
  return (
    <div className="rounded-md bg-muted/50 p-2.5">
      <p className="mb-1 text-xs text-muted-foreground">{t('command')}</p>
      <div className="flex items-start gap-2">
        <code className="min-w-0 flex-1 overflow-x-auto text-xs whitespace-pre-wrap break-all">
          {command}
        </code>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="shrink-0"
          onClick={() => {
            void navigator.clipboard.writeText(command).then(() => {
              setCopied(true)
            })
          }}
        >
          {copied ? <CheckIcon /> : <CopyIcon />}
          {copied ? t('copied') : t('copy')}
        </Button>
      </div>
    </div>
  )
}

export function FixAction({ fix }: { fix: Fix }) {
  const { t } = useTranslation('fixit')
  const isRoot = useIsRoot()
  const apply = useApplyFix()
  const [confirm, setConfirm] = useState(false)
  const [param, setParam] = useState(fix.param?.default ?? '')
  const [result, setResult] = useState<FixResult | null>(null)

  const auto = fix.kind === 'auto'
  const rateLimited = fix.state === 'rate_limited'
  const params = fix.param ? { [fix.param.name]: param } : undefined

  function run() {
    apply.mutate(
      { id: fix.id, params },
      {
        onSuccess: (r) => {
          setResult(r)
          setConfirm(false)
        },
      },
    )
  }

  return (
    <div className="mt-2 space-y-2 rounded-md border border-border p-3">
      <div className="flex flex-wrap items-center gap-2">
        <WrenchIcon className="size-4 shrink-0" aria-hidden="true" />
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

      {fix.command && !auto ? <CommandBlock command={fix.command} /> : null}

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
            <ArrowsClockwiseIcon />
            {apply.isPending ? t('rechecking') : t('recheck')}
          </Button>
        ) : null}
      </div>

      {apply.isError ? (
        <p role="alert" className="text-xs text-destructive">
          {apply.error.message}
        </p>
      ) : null}
      {result ? (
        <p role="status" className="text-xs text-foreground">
          {t(`outcome.${result.outcome}`, { detail: result.detail })}
        </p>
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

/** CheckFix renders the engine's fix for a failing check id, or nothing. */
export function CheckFix({ checkId }: { checkId: string }) {
  const fix = useFixForCheck(checkId)
  if (!fix) return null
  return <FixAction fix={fix} />
}
