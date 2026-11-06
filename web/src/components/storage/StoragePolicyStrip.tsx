import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, PencilGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { RoleGate } from '@/components/ui/RoleGate'
import { Switch } from '@/components/ui/switch'
import { useAbilities } from '@/hooks/useAbilities'
import type { JanitorPolicy } from '@/queries/janitor'
import {
  useSaveCleanupPolicy,
  useSaveStorageSetting,
  useStoragePolicy,
} from '@/queries/storagePolicy'
import {
  ageParts,
  buildChips,
  cleanupState,
  gbToMb,
  parsePositive,
  POLICY_KEYS,
  type ChipId,
  type PolicyChip,
} from './policyChips'

const CHIP_CLASS =
  'inline-flex min-h-8 items-center gap-1.5 rounded-lg border border-border bg-background px-2.5 py-1 text-left text-xs outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring aria-disabled:cursor-not-allowed aria-disabled:opacity-80'

interface EditorProps {
  onDone: () => void
}

function Footer({
  applied,
  error,
  pending,
  canSave,
  onSave,
  onDone,
}: {
  applied: boolean
  error: string | null
  pending: boolean
  canSave: boolean
  onSave: () => void
  onDone: () => void
}) {
  const { t } = useTranslation('storagePolicy')
  return (
    <div className="flex flex-col gap-2">
      {error ? (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      ) : null}
      <div className="flex items-center justify-end gap-2">
        {applied ? (
          <span
            role="status"
            className="mr-auto inline-flex items-center gap-1 text-xs text-success"
          >
            <CheckGlassIcon className="size-3.5" aria-hidden="true" />
            {t('applied')}
          </span>
        ) : null}
        <Button type="button" variant="ghost" size="sm" onClick={onDone}>
          {applied ? t('close') : t('cancel')}
        </Button>
        <Button
          type="button"
          size="sm"
          disabled={!canSave || pending}
          onClick={onSave}
        >
          {pending ? t('saving') : t('save')}
        </Button>
      </div>
    </div>
  )
}

function messageOf(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

interface NumberEditorProps extends EditorProps {
  id: Exclude<ChipId, 'cleanup'>
  initial: number
  whole: boolean
  save: (n: number) => Promise<unknown>
  pending: boolean
}

function NumberEditor({
  id,
  initial,
  whole,
  save,
  pending,
  onDone,
}: NumberEditorProps) {
  const { t } = useTranslation('storagePolicy')
  const [text, setText] = useState(String(initial))
  const [error, setError] = useState<string | null>(null)
  const [applied, setApplied] = useState(false)
  const parsed = parsePositive(text, whole)
  const invalidKey = whole ? 'invalid' : 'invalidGb'

  async function submit() {
    if (parsed === null) return
    setError(null)
    try {
      await save(parsed)
      setApplied(true)
    } catch (err) {
      setApplied(false)
      setError(messageOf(err))
    }
  }

  return (
    <>
      <Field>
        <FieldLabel className="text-[13px]">{t(`${id}.field`)}</FieldLabel>
        <div className="flex items-center gap-2">
          <Input
            inputMode={whole ? 'numeric' : 'decimal'}
            value={text}
            aria-invalid={parsed === null}
            className="h-8 w-24"
            onChange={(e) => {
              setText(e.target.value)
              setApplied(false)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') void submit()
            }}
          />
          <span className="text-xs text-muted-foreground">
            {t(`${id}.unit`)}
          </span>
        </div>
        <FieldDescription className="text-xs">
          {parsed === null ? t(invalidKey) : t('range')}
        </FieldDescription>
      </Field>
      <p className="text-xs text-muted-foreground">{t(`${id}.what`)}</p>
      <Footer
        applied={applied}
        error={error}
        pending={pending}
        canSave={parsed !== null}
        onSave={() => void submit()}
        onDone={onDone}
      />
    </>
  )
}

function CleanupEditor({
  policy,
  onDone,
}: EditorProps & { policy: JanitorPolicy }) {
  const { t } = useTranslation('storagePolicy')
  const { save, isPending } = useSaveCleanupPolicy()
  const [enabled, setEnabled] = useState(policy.enabled)
  const [dryRun, setDryRun] = useState(policy.dry_run)
  const [percent, setPercent] = useState(
    String(policy.disk_free_budget_percent),
  )
  const [error, setError] = useState<string | null>(null)
  const [applied, setApplied] = useState(false)
  const parsed = parsePositive(percent, false)

  async function submit() {
    if (parsed === null) return
    setError(null)
    try {
      await save(policy, {
        enabled,
        dry_run: dryRun,
        disk_free_budget_percent: parsed,
      })
      setApplied(true)
    } catch (err) {
      setApplied(false)
      setError(messageOf(err))
    }
  }

  return (
    <>
      <Field orientation="horizontal">
        <Switch
          size="sm"
          checked={enabled}
          aria-label={t('cleanup.enabled')}
          onCheckedChange={(v) => {
            setEnabled(v)
            setApplied(false)
          }}
        />
        <FieldLabel className="text-[13px]">{t('cleanup.enabled')}</FieldLabel>
      </Field>
      <Field orientation="horizontal">
        <Switch
          size="sm"
          checked={dryRun}
          aria-label={t('cleanup.dryRunField')}
          onCheckedChange={(v) => {
            setDryRun(v)
            setApplied(false)
          }}
        />
        <FieldLabel className="text-[13px]">
          {t('cleanup.dryRunField')}
        </FieldLabel>
      </Field>
      <Field>
        <FieldLabel className="text-[13px]">{t('cleanup.budget')}</FieldLabel>
        <div className="flex items-center gap-2">
          <Input
            inputMode="decimal"
            value={percent}
            aria-invalid={parsed === null}
            className="h-8 w-24"
            onChange={(e) => {
              setPercent(e.target.value)
              setApplied(false)
            }}
          />
          <span className="text-xs text-muted-foreground">
            {t('cleanup.budgetUnit')}
          </span>
        </div>
        <FieldDescription className="text-xs">
          {parsed === null ? t('invalid') : t('range')}
        </FieldDescription>
      </Field>
      <p className="text-xs text-muted-foreground">{t('cleanup.what')}</p>
      <Footer
        applied={applied}
        error={error}
        pending={isPending}
        canSave={parsed !== null}
        onSave={() => void submit()}
        onDone={onDone}
      />
    </>
  )
}

function ChipFace({ title, value }: { title: string; value: string }) {
  return (
    <>
      <span className="text-muted-foreground">{title}</span>
      <span className="font-medium text-foreground">{value}</span>
    </>
  )
}

function Chip({
  title,
  value,
  editable,
  children,
}: {
  title: string
  value: string
  editable: boolean
  children: (close: () => void) => ReactNode
}) {
  const [open, setOpen] = useState(false)
  if (!editable) {
    return (
      <RoleGate ability="root">
        <button type="button" className={CHIP_CLASS}>
          <ChipFace title={title} value={value} />
        </button>
      </RoleGate>
    )
  }
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger render={<button type="button" className={CHIP_CLASS} />}>
        <ChipFace title={title} value={value} />
        <PencilGlassIcon
          className="size-3.5 text-muted-foreground"
          aria-hidden="true"
        />
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80">
        {open ? children(() => setOpen(false)) : null}
      </PopoverContent>
    </Popover>
  )
}

/** StoragePolicyStrip shows the live cleanup and disk policy as chips with small admin-only editors. */
export function StoragePolicyStrip() {
  const { t } = useTranslation('storagePolicy')
  const { can } = useAbilities()
  const { janitor, settings, loading } = useStoragePolicy()
  const setting = useSaveStorageSetting()
  const cleanup = useSaveCleanupPolicy()
  const editable = can('root')
  if (loading) return null
  const chips = buildChips(janitor, settings)

  function valueText(chip: PolicyChip): string {
    if (chip.value === null) return '-'
    switch (chip.id) {
      case 'cleanup': {
        const state = cleanupState(janitor as JanitorPolicy)
        const base = t(`cleanup.${state}`)
        return state === 'off'
          ? base
          : `${base}, ${t('cleanup.budgetValue', { percent: chip.value })}`
      }
      case 'keepImages':
        return t('keepImages.value', { count: chip.value })
      case 'cacheAge': {
        const { unit, n } = ageParts(chip.value)
        return t(`cacheAge.${unit}`, { count: n })
      }
      case 'lowDisk':
        return t('lowDisk.value', { gb: chip.value })
      case 'criticalDisk':
        return t('criticalDisk.value', { gb: chip.value })
    }
  }

  function editor(chip: PolicyChip, close: () => void): ReactNode {
    const initial = chip.value ?? 1
    switch (chip.id) {
      case 'cleanup':
        return (
          <CleanupEditor policy={janitor as JanitorPolicy} onDone={close} />
        )
      case 'keepImages':
        return (
          <NumberEditor
            id="keepImages"
            initial={initial}
            whole
            pending={setting.isPending}
            save={(n) => setting.save(POLICY_KEYS.keepPerApp, n)}
            onDone={close}
          />
        )
      case 'cacheAge':
        return (
          <NumberEditor
            id="cacheAge"
            initial={initial}
            whole
            pending={cleanup.isPending}
            save={(n) =>
              cleanup.save(janitor as JanitorPolicy, {
                build_cache_max_age_hours: n,
              })
            }
            onDone={close}
          />
        )
      case 'lowDisk':
        return (
          <NumberEditor
            id="lowDisk"
            initial={initial}
            whole={false}
            pending={setting.isPending}
            save={(n) => setting.save(POLICY_KEYS.lowFreeMb, gbToMb(n))}
            onDone={close}
          />
        )
      case 'criticalDisk':
        return (
          <NumberEditor
            id="criticalDisk"
            initial={initial}
            whole={false}
            pending={setting.isPending}
            save={(n) => setting.save(POLICY_KEYS.criticalFreeMb, gbToMb(n))}
            onDone={close}
          />
        )
    }
  }

  return (
    <section
      aria-label={t('label')}
      className="flex flex-wrap items-center gap-2"
    >
      {chips.map((chip) => (
        <Chip
          key={chip.id}
          title={t(`${chip.id}.title`)}
          value={valueText(chip)}
          editable={editable && chip.value !== null}
        >
          {(close) => editor(chip, close)}
        </Chip>
      ))}
    </section>
  )
}
