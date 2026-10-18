import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  isSelectableItem,
  type MigrateFix,
  type MigrateItem,
  type MigrateOverride,
  type MigrateVerdict,
} from '../../queries/migrateWizard'

const VERDICT_VARIANT: Record<
  MigrateVerdict,
  'success' | 'info' | 'warning' | 'destructive' | 'muted'
> = {
  will_import: 'success',
  imports_with_changes: 'info',
  needs_manual: 'warning',
  unsupported: 'destructive',
  already_imported: 'muted',
}

interface Props {
  item: MigrateItem
  selected: boolean
  busy: boolean
  onToggle: () => void
  onFix: (patch: MigrateOverride) => void
  onOpenCutover: () => void
}

function RenameFix({
  initial,
  busy,
  onApply,
}: {
  initial: string
  busy: boolean
  onApply: (name: string) => void
}) {
  const { t } = useTranslation('migrate')
  const [name, setName] = useState(initial)
  return (
    <form
      className="flex items-center gap-2"
      onSubmit={(e) => {
        e.preventDefault()
        if (name.trim()) onApply(name.trim())
      }}
    >
      <Input
        aria-label={t('fix.rename')}
        placeholder={t('fix.renamePlaceholder')}
        value={name}
        onChange={(e) => setName(e.target.value)}
        className="w-48"
      />
      <Button type="submit" size="sm" variant="outline" disabled={busy}>
        {t('fix.rename')}
      </Button>
    </form>
  )
}

function PortFix({
  busy,
  onApply,
}: {
  busy: boolean
  onApply: (port: number) => void
}) {
  const { t } = useTranslation('migrate')
  const [port, setPort] = useState('')
  const n = Number(port)
  const valid = Number.isInteger(n) && n > 0 && n < 65536
  return (
    <form
      className="flex items-center gap-2"
      onSubmit={(e) => {
        e.preventDefault()
        if (valid) onApply(n)
      }}
    >
      <Input
        aria-label={t('fix.setPort')}
        placeholder={t('fix.portPlaceholder')}
        inputMode="numeric"
        value={port}
        onChange={(e) => setPort(e.target.value)}
        className="w-24"
      />
      <Button
        type="submit"
        size="sm"
        variant="outline"
        disabled={busy || !valid}
      >
        {t('fix.setPort')}
      </Button>
    </form>
  )
}

function FixControl({
  fix,
  item,
  busy,
  onFix,
  onOpenCutover,
}: {
  fix: MigrateFix
  item: MigrateItem
  busy: boolean
  onFix: (patch: MigrateOverride) => void
  onOpenCutover: () => void
}) {
  const { t } = useTranslation('migrate')
  switch (fix.code) {
    case 'rename':
      return (
        <RenameFix
          initial={item.target ?? item.name}
          busy={busy}
          onApply={(name) => onFix({ name })}
        />
      )
    case 'set_port':
      return <PortFix busy={busy} onApply={(port) => onFix({ port })} />
    case 'hold_domains':
      return (
        <div className="flex flex-wrap items-center gap-2">
          <Button
            size="sm"
            variant="outline"
            disabled={busy}
            onClick={() => onFix({ drop_domains: true })}
          >
            {t('fix.holdDomains')}
          </Button>
          <span className="text-xs text-muted-foreground">
            {t('fix.holdDomainsHint', { domains: fix.detail })}
          </span>
        </div>
      )
    case 'copy_data':
      return (
        <Button
          size="sm"
          variant="outline"
          render={<Link to="/settings/import-platform" />}
          nativeButton={false}
        >
          {t('fix.copyData')}
        </Button>
      )
    case 'copy_volumes':
      return (
        <Button size="sm" variant="outline" onClick={onOpenCutover}>
          {t('fix.copyVolumes')}
        </Button>
      )
    case 'use_template':
      return (
        <Button
          size="sm"
          variant="outline"
          render={<Link to="/templates" />}
          nativeButton={false}
        >
          {t('fix.useTemplate')}
        </Button>
      )
    default:
      return null
  }
}

export function MigrateItemRow({
  item,
  selected,
  busy,
  onToggle,
  onFix,
  onOpenCutover,
}: Props) {
  const { t } = useTranslation('migrate')
  const selectable = isSelectableItem(item)
  const fixes = (item.fixes ?? []).filter(
    (f) => f.kind !== 'validate' && f.code !== 'connect_repo',
  )
  const p = item.preview
  return (
    <div className="space-y-2 border-b px-3 py-3 last:border-b-0">
      <div className="flex flex-wrap items-center gap-2">
        <Checkbox
          aria-label={t('item.select', { name: item.name })}
          checked={selected}
          disabled={!selectable}
          onCheckedChange={onToggle}
        />
        <span className="font-medium">{item.name}</span>
        <Badge variant="outline">
          {t(`kind.${item.kind}`, { defaultValue: item.kind })}
        </Badge>
        <Badge variant={VERDICT_VARIANT[item.verdict]}>
          {t(`verdict.${item.verdict}`)}
        </Badge>
        {item.target && item.target !== item.name ? (
          <span className="text-xs text-muted-foreground">
            {'-> '}
            {item.target}
          </span>
        ) : null}
      </div>
      <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
        {item.source ? <span>{item.source}</span> : null}
        {item.project ? <span>{item.project}</span> : null}
        {item.port ? <span>{t('item.port', { port: item.port })}</span> : null}
        {item.kind === 'app' ? (
          <>
            <span>{t('item.env', { count: item.env_count })}</span>
            <span>
              {t('item.volumes', { count: item.volumes?.length ?? 0 })}
            </span>
            <span>
              {t('item.domains', { count: item.domains?.length ?? 0 })}
            </span>
            <span>{t('item.tasks', { count: item.crons?.length ?? 0 })}</span>
          </>
        ) : null}
      </div>
      {(item.reasons ?? []).length > 0 ? (
        <ul className="list-disc space-y-0.5 pl-5 text-sm">
          {item.reasons?.map((r) => (
            <li key={r}>{r}</li>
          ))}
        </ul>
      ) : null}
      {(item.manual ?? []).map((m) => (
        <p key={m} className="text-sm text-muted-foreground">
          <span className="font-medium text-foreground">
            {t('item.manual')}:
          </span>{' '}
          {m}
        </p>
      ))}
      {p ? (
        <div className="rounded-md bg-muted/50 px-3 py-2 text-xs">
          <p className="font-medium">{t('item.preview')}</p>
          <p>
            {p.engine
              ? t('item.previewDatabase', {
                  name: p.name,
                  engine: p.engine,
                  version: p.version,
                })
              : t('item.previewApp', {
                  name: p.name,
                  image: p.image,
                  port: p.port,
                  env: p.env,
                  secrets: p.secrets,
                  volumes: p.volumes,
                })}
          </p>
          {p.domains && p.domains.length > 0 ? (
            <p>{t('item.previewDomains', { domains: p.domains.join(', ') })}</p>
          ) : null}
          {p.dropped_keys && p.dropped_keys.length > 0 ? (
            <p className="text-muted-foreground">
              {t('item.dropped', { keys: p.dropped_keys.join(', ') })}
            </p>
          ) : null}
        </div>
      ) : null}
      {fixes.length > 0 ? (
        <div className="flex flex-wrap items-center gap-3">
          {fixes.map((f) => (
            <FixControl
              key={f.code}
              fix={f}
              item={item}
              busy={busy}
              onFix={onFix}
              onOpenCutover={onOpenCutover}
            />
          ))}
        </div>
      ) : null}
    </div>
  )
}
