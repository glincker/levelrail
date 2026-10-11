import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowRightIcon,
  FloppyDiskIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { diffFields } from './usePolicyDraft'
import type { PreviewStep } from '../../queries/domainPolicyTypes'

export const selectClass =
  'h-8 rounded-md border border-input bg-background px-2 text-sm focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none'

export function NativeSelect({
  id,
  label,
  value,
  options,
  onChange,
  className,
}: {
  id: string
  label: string
  value: string
  options: { value: string; label: string }[]
  onChange: (v: string) => void
  className?: string
}) {
  return (
    <label
      htmlFor={id}
      className={cn('flex flex-col gap-1 text-xs', className)}
    >
      <span className="text-muted-foreground">{label}</span>
      <select
        id={id}
        className={selectClass}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      >
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </label>
  )
}

export function FieldMessage({ message }: { message?: string }) {
  if (!message) return null
  return (
    <p role="alert" className="text-xs text-destructive">
      {message}
    </p>
  )
}

export function CheckboxField({
  id,
  label,
  checked,
  onChange,
}: {
  id: string
  label: string
  checked: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <label htmlFor={id} className="flex items-center gap-2 text-sm">
      <input
        id={id}
        type="checkbox"
        className="size-4 accent-primary"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
      />
      {label}
    </label>
  )
}

// SaveBar shows what changes before the operator commits it.
export function SaveBar({
  saved,
  draft,
  dirty,
  saving,
  error,
  onSave,
  onDiscard,
}: {
  saved: unknown
  draft: unknown
  dirty: boolean
  saving: boolean
  error?: string | null
  onSave: () => void
  onDiscard: () => void
}) {
  const { t } = useTranslation('domainPolicies')
  const [showDiff, setShowDiff] = useState(false)
  const diff = dirty ? diffFields(saved, draft) : []
  return (
    <div className="space-y-2 rounded-md border border-border bg-muted/30 p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground" aria-live="polite">
          {dirty ? t('save.unsaved', { count: diff.length }) : t('save.clean')}
        </p>
        <div className="flex gap-2">
          {dirty ? (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => setShowDiff((v) => !v)}
              aria-expanded={showDiff}
            >
              {showDiff ? t('save.hideDiff') : t('save.showDiff')}
            </Button>
          ) : null}
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={!dirty || saving}
            onClick={onDiscard}
          >
            {t('save.discard')}
          </Button>
          <Button
            type="button"
            size="sm"
            disabled={!dirty || saving}
            onClick={onSave}
          >
            <FloppyDiskIcon />
            {saving ? t('save.saving') : t('save.save')}
          </Button>
        </div>
      </div>
      {showDiff && diff.length > 0 ? (
        <ul
          className="space-y-1 font-mono text-xs"
          aria-label={t('save.diffLabel')}
        >
          {diff.map((d) => (
            <li key={d.path} className="flex flex-wrap items-center gap-1">
              <span className="text-foreground">{d.path}</span>
              <span className="text-destructive line-through">
                {d.before || t('save.none')}
              </span>
              <ArrowRightIcon className="size-3" aria-hidden="true" />
              <span className="text-green-700 dark:text-green-400">
                {d.after || t('save.none')}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
      {error ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
    </div>
  )
}

export function PreviewPanel({
  steps,
  loading,
  errors,
}: {
  steps?: PreviewStep[]
  loading: boolean
  errors?: { field: string; message: string }[]
}) {
  const { t } = useTranslation('domainPolicies')
  return (
    <section
      aria-label={t('preview.title')}
      className="space-y-2 rounded-md border border-border p-3"
    >
      <h3 className="text-sm font-medium">{t('preview.title')}</h3>
      <p className="text-xs text-muted-foreground">{t('preview.help')}</p>
      {loading && !steps ? (
        <p className="text-xs text-muted-foreground">{t('preview.loading')}</p>
      ) : null}
      <ol className="space-y-1 text-sm">
        {(steps ?? []).map((s, i) => (
          <li key={`${s.stage}-${i}`} className="flex gap-2">
            <span className="w-24 shrink-0 font-mono text-xs text-muted-foreground uppercase">
              {s.stage}
            </span>
            <span className={s.final ? 'font-medium' : undefined}>
              {s.outcome}
            </span>
          </li>
        ))}
      </ol>
      {errors && errors.length > 0 ? (
        <ul className="space-y-1 text-xs text-destructive">
          {errors.map((e) => (
            <li key={e.field + e.message}>
              {e.field ? `${e.field}: ` : ''}
              {e.message}
            </li>
          ))}
        </ul>
      ) : null}
    </section>
  )
}

export function Section({
  title,
  description,
  children,
}: {
  title: string
  description?: string
  children: ReactNode
}) {
  return (
    <section className="space-y-3 rounded-lg border border-border bg-card p-4">
      <div>
        <h3 className="text-sm font-semibold">{title}</h3>
        {description ? (
          <p className="text-xs text-muted-foreground">{description}</p>
        ) : null}
      </div>
      {children}
    </section>
  )
}
