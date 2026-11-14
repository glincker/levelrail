import { useTranslation } from 'react-i18next'
import type {
  IacChange,
  IacFieldChange,
  IacItemResult,
  IacPlan,
} from '../queries/iac'
import { CodeBlock } from '@/components/ui/code-block'
import { labelOf } from '../lib/labelOf'
import { IAC_CODE_KEYS } from '../lib/iacPlanCodes'
import { IacCodeBadge } from './iac/IacCodeBadge'

const actionStyle: Record<string, string> = {
  create: 'text-green-700 dark:text-green-400',
  update: 'text-amber-700 dark:text-amber-400',
  delete: 'text-red-700 dark:text-red-400',
  error: 'text-red-700 dark:text-red-400',
  noop: 'text-muted-foreground',
}

const actionMark: Record<string, string> = {
  create: '+',
  update: '~',
  delete: '-',
  error: '!',
  noop: ' ',
}

function fieldText(f: IacFieldChange, keptNote: string): string {
  switch (f.op) {
    case 'add':
      return `+ ${f.path}: ${f.new ?? ''}`
    case 'remove':
      return `- ${f.path}: ${f.old ?? ''}`
    case 'keep':
      return `= ${f.path} (${keptNote})`
    default:
      return `~ ${f.path}: ${f.old ?? ''} -> ${f.new ?? ''}`
  }
}

function changeKey(c: IacChange): string {
  return [c.kind, c.scope, c.name].filter(Boolean).join('/')
}

export function IacSummaryLine({ plan }: { plan: IacPlan }) {
  const { t } = useTranslation('settings')
  const s = plan.summary
  return (
    <p className="text-sm text-foreground">
      {t(s.error > 0 ? 'iacPlan.summaryErrors' : 'iacPlan.summary', {
        create: s.create,
        update: s.update,
        delete: s.delete,
        noop: s.noop,
        error: s.error,
      })}
    </p>
  )
}

function CodeHint({ code }: { code: string }) {
  const { t } = useTranslation('settings')
  const base = labelOf(IAC_CODE_KEYS, code)
  // An unknown code resolves to itself, so only the raw badge shows.
  const known = base !== code
  return (
    <div className="mt-1 flex flex-wrap items-baseline gap-x-2 gap-y-1 text-sm">
      <IacCodeBadge code={code} />
      {known ? (
        <>
          <span className="text-foreground">{t(`${base}.message`)}</span>
          <span className="text-muted-foreground">
            {t('iacPlan.fix', { hint: t(`${base}.fix`) })}
          </span>
        </>
      ) : null}
    </div>
  )
}

function ChangeRow({ change }: { change: IacChange }) {
  const { t } = useTranslation('settings')
  return (
    <li className="rounded-md border border-border p-3">
      <div className="flex flex-wrap items-baseline gap-2 font-mono text-sm">
        <span className={labelOf(actionStyle, change.action)}>
          {Object.hasOwn(actionMark, change.action)
            ? actionMark[change.action]
            : '?'}
        </span>
        <span
          className={`rounded border px-1.5 text-xs ${labelOf(actionStyle, change.action)}`}
        >
          {t(`iacStudio.planAction.${change.action}`, {
            defaultValue: change.action,
          })}
        </span>
        <span className="text-foreground">{changeKey(change)}</span>
        {change.file ? (
          <span className="text-xs text-muted-foreground">
            {change.file}:{change.line}
          </span>
        ) : null}
        {change.denied ? (
          <span className="text-xs text-red-700 dark:text-red-400">
            {t('iacPlan.denied')}
          </span>
        ) : null}
      </div>
      {change.code ? <CodeHint code={change.code} /> : null}
      {change.reason ? (
        <p className="mt-1 text-sm text-red-700 dark:text-red-400">
          {change.reason}
        </p>
      ) : null}
      {change.fields?.length || change.kept?.length ? (
        <CodeBlock
          className="mt-2"
          code={[...(change.fields ?? []), ...(change.kept ?? [])]
            .map((f) => fieldText(f, t('iacPlan.keptNote')))
            .join('\n')}
          language="diff"
          wrap
        />
      ) : null}
      {change.warnings?.map((w) => (
        <p key={w} className="mt-1 text-xs text-amber-700 dark:text-amber-400">
          {t('iacPlan.warning', { message: w })}
        </p>
      ))}
    </li>
  )
}

export function IacPlanView({ plan }: { plan: IacPlan }) {
  const { t } = useTranslation('settings')
  const shown = plan.changes.filter((c) => c.action !== 'noop')
  return (
    <div className="space-y-3">
      <IacSummaryLine plan={plan} />
      {shown.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('iacPlan.matches')}</p>
      ) : (
        <ul className="space-y-2">
          {shown.map((c) => (
            <ChangeRow key={changeKey(c) + c.action} change={c} />
          ))}
        </ul>
      )}
    </div>
  )
}

const statusStyle: Record<string, string> = {
  applied: 'text-green-700 dark:text-green-400',
  failed: 'text-red-700 dark:text-red-400',
  denied: 'text-red-700 dark:text-red-400',
  skipped: 'text-amber-700 dark:text-amber-400',
  noop: 'text-muted-foreground',
}

export function IacResultView({ results }: { results: IacItemResult[] }) {
  const { t } = useTranslation('settings')
  const shown = results.filter((r) => r.status !== 'noop')
  return (
    <ul className="space-y-1.5">
      {shown.map((r) => (
        <li key={changeKey(r) + r.action} className="font-mono text-sm">
          <span className={labelOf(statusStyle, r.status)}>
            {t(`iacPlan.status.${r.status}`, { defaultValue: r.status })}
          </span>{' '}
          <span className="text-foreground">{changeKey(r)}</span>
          {r.error ? (
            <span className="text-muted-foreground">: {r.error}</span>
          ) : null}
        </li>
      ))}
    </ul>
  )
}
