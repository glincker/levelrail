import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { CopyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { messageOf } from '../../lib/cloudAwsServices'
import { COPY_FEEDBACK_MS } from '../../lib/copyFeedback'

export function CopyButton({
  text,
  label,
  getText,
}: Readonly<{
  label: string
  text?: string
  getText?: () => Promise<string>
}>) {
  const { t } = useTranslation('cloudAccounts')
  const [done, setDone] = useState(false)
  const [busy, setBusy] = useState(false)
  const run = async () => {
    setBusy(true)
    try {
      const value = getText ? await getText() : (text ?? '')
      await navigator.clipboard.writeText(value)
      setDone(true)
      window.setTimeout(() => {
        setDone(false)
      }, COPY_FEEDBACK_MS)
    } catch {
      setDone(false)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Button
      size="sm"
      variant="outline"
      disabled={busy}
      aria-label={label}
      onClick={() => {
        void run()
      }}
    >
      <CopyGlassIcon size="1.2em" />
      {done ? t('services.copied') : label}
    </Button>
  )
}

export function ServiceShell({
  id,
  title,
  enabled,
  canToggle,
  toggling,
  onToggle,
  children,
}: Readonly<{
  id: string
  title: string
  enabled: boolean
  canToggle: boolean
  toggling: boolean
  onToggle: (on: boolean) => void
  children: ReactNode
}>) {
  const { t } = useTranslation('cloudAccounts')
  return (
    <section
      aria-labelledby={id}
      className="space-y-2 rounded-lg border border-border p-3"
    >
      <div className="flex items-center gap-2">
        <h3 id={id} className="text-sm font-semibold">
          {title}
        </h3>
        <span className="ml-auto text-xs text-muted-foreground">
          {enabled ? t('services.on') : t('services.off')}
        </span>
        <Switch
          size="sm"
          checked={enabled}
          disabled={!canToggle || toggling}
          aria-label={t('services.toggle', { name: title })}
          onCheckedChange={onToggle}
        />
      </div>
      {enabled ? (
        children
      ) : (
        <p className="text-sm text-muted-foreground">
          {canToggle ? t('services.disabledRoot') : t('services.disabled')}
        </p>
      )}
    </section>
  )
}

export function StateLine({
  pending,
  error,
  empty,
}: Readonly<{ pending: boolean; error: unknown; empty?: string }>) {
  const { t } = useTranslation('cloudAccounts')
  if (pending) {
    return (
      <p role="status" className="text-sm text-muted-foreground">
        {t('section.loading')}
      </p>
    )
  }
  if (error) {
    return (
      <p role="alert" className="text-sm text-destructive">
        {messageOf(error)}
      </p>
    )
  }
  return empty ? <p className="text-sm text-muted-foreground">{empty}</p> : null
}
