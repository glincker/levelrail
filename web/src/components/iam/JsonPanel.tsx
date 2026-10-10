import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  CircleNotchIcon,
  LockSimpleIcon,
  WarningCircleIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import { TONE } from '../kit/tone'
import type { PolicyDocument } from '../../queries/iamPolicies'
import type { FieldIssue, IamFinding } from '../../queries/iamBuilder'

const PLANNED_KEYS = ['sourceIp', 'timeWindow', 'mfa'] as const

/** JsonPanel is the live document beside the guided rules. It is read-only until the advanced switch is on, and lists problems by field path. */
export function JsonPanel({
  advanced,
  onAdvancedChange,
  guidedDocument,
  jsonText,
  onJsonTextChange,
  jsonError,
  issues,
  findings,
  checking,
}: {
  advanced: boolean
  onAdvancedChange: (on: boolean) => void
  guidedDocument: PolicyDocument
  jsonText: string
  onJsonTextChange: (text: string) => void
  jsonError: string | null
  issues: FieldIssue[]
  findings: IamFinding[]
  checking: boolean
}) {
  const { t } = useTranslation('iam')
  const valid = !jsonError && !issues.some((i) => i.severity === 'error')
  return (
    <section aria-label={t('builder.json.title')} className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-sm font-medium text-foreground">
          {t('builder.json.title')}
        </h3>
        <label className="flex items-center gap-2 text-xs text-muted-foreground">
          {t('builder.json.advanced')}
          <Switch
            checked={advanced}
            onCheckedChange={onAdvancedChange}
            aria-label={t('builder.json.advanced')}
          />
        </label>
      </div>
      <Textarea
        aria-label={t('builder.json.title')}
        readOnly={!advanced}
        spellCheck={false}
        rows={14}
        className="font-mono text-xs"
        value={advanced ? jsonText : JSON.stringify(guidedDocument, null, 2)}
        onChange={(e) => onJsonTextChange(e.target.value)}
      />
      {jsonError ? (
        <p className="text-xs text-destructive" role="alert">
          {t('builder.json.invalid', { reason: jsonError })}
        </p>
      ) : null}

      <div aria-live="polite" className="space-y-1.5">
        <p
          className={cn(
            'flex items-center gap-1.5 text-xs',
            valid ? TONE.success.text : 'text-muted-foreground',
          )}
        >
          {checking ? (
            <CircleNotchIcon className="size-3.5 animate-spin motion-reduce:animate-none" />
          ) : valid ? (
            <CheckCircleIcon className="size-3.5" />
          ) : (
            <XCircleIcon className="size-3.5 text-destructive" />
          )}
          {checking
            ? t('builder.issues.checking')
            : valid
              ? t('builder.issues.ok')
              : t('builder.issues.title')}
        </p>
        <ul className="space-y-1">
          {issues.map((i) => (
            <li
              key={`${i.path}:${i.message}`}
              className={cn(
                'text-xs',
                i.severity === 'error' ? 'text-destructive' : TONE.warning.text,
              )}
            >
              <code className="mr-1 text-muted-foreground">{i.path}</code>
              {i.message}
            </li>
          ))}
          {findings.map((f) => (
            <li
              key={`${f.kind}:${f.message}`}
              className="flex items-start gap-1.5 text-xs text-muted-foreground"
            >
              <WarningCircleIcon
                className={cn('mt-0.5 size-3.5 shrink-0', TONE.warning.text)}
              />
              <span>
                {f.message} <span className="text-foreground">{f.fix}</span>
              </span>
            </li>
          ))}
        </ul>
      </div>

      <div className="space-y-1.5 rounded-lg border border-dashed border-border p-3">
        <h4 className="flex items-center gap-1.5 text-xs font-medium text-foreground">
          <LockSimpleIcon className="size-3.5" aria-hidden="true" />
          {t('builder.conditions.title')}
        </h4>
        <p className="text-xs text-muted-foreground">
          {t('builder.conditions.description')}
        </p>
        <ul className="flex flex-wrap gap-1.5">
          {PLANNED_KEYS.map((k) => (
            <li
              key={k}
              className="rounded-full border border-border px-2 py-0.5 text-xs text-muted-foreground"
              title={t('builder.conditions.planned')}
            >
              {t(`builder.conditions.${k}`)}
            </li>
          ))}
        </ul>
      </div>
    </section>
  )
}
