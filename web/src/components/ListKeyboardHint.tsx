import { useTranslation } from 'react-i18next'
import { KeyboardGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'

const KEYS = [
  ['j', 'down'],
  ['k', 'up'],
  ['x', 'select'],
  ['Enter', 'open'],
  ['/', 'filter'],
  ['?', 'hint'],
] as const

/** ListKeyboardHint lists the list shortcuts that useListKeyboard handles; toggled with the ? key. */
export function ListKeyboardHint({
  open,
  onClose,
}: {
  open: boolean
  onClose: () => void
}) {
  const { t } = useTranslation('uiShared')
  if (!open) return null
  return (
    <div
      role="note"
      aria-label={t('listKeys.title')}
      className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-glinr-control border border-border bg-card px-3 py-2 text-xs text-muted-foreground"
    >
      <KeyboardGlassIcon size="1.4em" aria-hidden="true" />
      {KEYS.map(([key, action]) => (
        <span key={key} className="inline-flex items-center gap-1">
          <kbd className="rounded border border-border bg-muted px-1.5 font-mono text-foreground">
            {key}
          </kbd>
          {t(`listKeys.${action}`)}
        </span>
      ))}
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        className="ml-auto"
        aria-label={t('listKeys.close')}
        onClick={onClose}
      >
        <XGlassIcon size="1.4em" />
      </Button>
    </div>
  )
}
