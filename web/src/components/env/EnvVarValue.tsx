import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CopyGlassIcon,
  EyeGlassIcon,
  EyeOffGlassIcon,
  PencilGlassIcon,
  RotateCcwGlassIcon,
} from '@thesvg/ui-react'
import { MASKED_VALUE, type EnvRow } from '../../lib/envRows'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

export interface EnvVarValueProps {
  row: EnvRow
  revealed?: string
  expiresAt?: number
  canReveal: boolean
  onChange: (row: EnvRow) => void
  onReveal: () => void
  onHide: () => void
  onCopy: (value: string) => void
}

function Countdown({ until }: { until: number }) {
  const { t } = useTranslation('envEditor')
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = window.setInterval(() => {
      setNow(Date.now())
    }, 1000)
    return () => {
      window.clearInterval(id)
    }
  }, [])
  const seconds = Math.max(0, Math.ceil((until - now) / 1000))
  return (
    <span className="text-xs text-muted-foreground tabular-nums">
      {t('value.hidesIn', { seconds })}
    </span>
  )
}

function Masked({ label }: { label: string }) {
  return (
    <span className="flex min-w-0 items-center gap-2">
      <span
        aria-hidden
        className="font-mono tracking-widest text-muted-foreground"
      >
        {MASKED_VALUE}
      </span>
      <span className="sr-only">{label}</span>
    </span>
  )
}

export function EnvVarValue({
  row,
  revealed,
  expiresAt,
  canReveal,
  onChange,
  onReveal,
  onHide,
  onCopy,
}: EnvVarValueProps) {
  const { t } = useTranslation('envEditor')
  const isNew = !row.original
  const storedHidden =
    !isNew &&
    (row.type === 'secret' ? row.value === '' : row.value === MASKED_VALUE)
  const replacing =
    !isNew &&
    ((row.type === 'secret' && row.value !== '') ||
      (row.type === 'plain' &&
        row.original?.value === MASKED_VALUE &&
        row.value !== MASKED_VALUE))

  if (row.type === 'vault') {
    const ref = row.vault ?? { path: '', key: '' }
    return (
      <div className="grid min-w-0 grid-cols-[minmax(0,2fr)_minmax(0,1fr)] gap-2">
        <Input
          aria-label={`${t('value.vaultPath')} ${row.key}`}
          placeholder={t('value.vaultPath')}
          className="font-mono"
          value={ref.path}
          onChange={(e) => {
            onChange({ ...row, vault: { ...ref, path: e.target.value } })
          }}
        />
        <Input
          aria-label={`${t('value.vaultKey')} ${row.key}`}
          placeholder={t('value.vaultKey')}
          className="font-mono"
          value={ref.key}
          onChange={(e) => {
            onChange({ ...row, vault: { ...ref, key: e.target.value } })
          }}
        />
      </div>
    )
  }

  if (
    replacing ||
    isNew ||
    (row.type === 'plain' && !row.sensitive && row.value !== MASKED_VALUE)
  ) {
    return (
      <div className="flex min-w-0 items-center gap-1">
        <Input
          aria-label={`${t('columns.value')} ${row.key}`}
          type={row.type === 'secret' || row.sensitive ? 'password' : 'text'}
          autoComplete="off"
          spellCheck={false}
          placeholder={
            replacing
              ? t('value.replacePlaceholder')
              : t('value.valuePlaceholder')
          }
          className="font-mono"
          value={row.value === MASKED_VALUE ? '' : row.value}
          onChange={(e) => {
            onChange({ ...row, value: e.target.value })
          }}
        />
        {replacing ? (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={t('actions.cancelReplace')}
            onClick={() => {
              onChange({ ...row, value: row.original?.value ?? '' })
            }}
          >
            <RotateCcwGlassIcon size="1.4em" />
          </Button>
        ) : null}
      </div>
    )
  }

  if (revealed !== undefined) {
    return (
      <div className="flex min-w-0 flex-wrap items-center gap-1">
        <code className="min-w-0 flex-1 truncate rounded-md bg-muted px-2 py-1 font-mono text-sm">
          {revealed}
        </code>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={`${t('actions.copy')} ${row.key}`}
          onClick={() => {
            onCopy(revealed)
          }}
        >
          <CopyGlassIcon size="1.4em" />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={`${t('actions.hide')} ${row.key}`}
          onClick={onHide}
        >
          <EyeOffGlassIcon size="1.4em" />
        </Button>
        {expiresAt ? <Countdown until={expiresAt} /> : null}
      </div>
    )
  }

  return (
    <div className="flex min-w-0 flex-wrap items-center gap-1">
      {storedHidden ? (
        <Masked label={t('value.masked')} />
      ) : (
        <span className="truncate font-mono text-sm text-muted-foreground">
          {t('value.empty')}
        </span>
      )}
      {storedHidden && canReveal ? (
        <Button type="button" variant="ghost" size="sm" onClick={onReveal}>
          <EyeGlassIcon size="1.4em" />
          {t('actions.reveal')}
        </Button>
      ) : null}
      {row.origin === 'own' ? (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => {
            onChange({ ...row, value: '' })
          }}
        >
          <PencilGlassIcon size="1.4em" />
          {t('actions.replace')}
        </Button>
      ) : null}
    </div>
  )
}
