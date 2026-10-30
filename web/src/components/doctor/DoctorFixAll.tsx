import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { WrenchGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { FixCli } from '@/components/fixit/FixCli'
import { useBrand } from '../../hooks/useBrand'
import { useIsRoot } from '../../hooks/useIsRoot'
import { cliCommand } from '../../lib/cliCommand'
import { safeAutoFixes } from '../../lib/doctorModel'
import { formatBytes } from '../../lib/format'
import { useApplyFix, useFixes } from '../../queries/fixit'

/** DoctorFixAll applies every safe one-click fix after a confirmation that lists exactly what changes. */
export function DoctorFixAll({ onDone }: { onDone: () => void }) {
  const { t } = useTranslation('settings')
  const brand = useBrand()
  const isRoot = useIsRoot()
  const fixes = useFixes()
  const apply = useApplyFix()
  const [open, setOpen] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [pending, setPending] = useState(false)
  const safe = safeAutoFixes(fixes.data ?? [])
  if (safe.length === 0) return null

  const consequences = safe.flatMap((f) => [
    f.title,
    ...f.changes.map(
      (c) =>
        `  ${c.label}${c.detail ? `: ${c.detail}` : ''}${c.bytes ? ` (${formatBytes(c.bytes)})` : ''}`,
    ),
  ])

  async function run() {
    setPending(true)
    setError(null)
    try {
      for (const f of safe) {
        await apply.mutateAsync({ id: f.id })
      }
      setOpen(false)
      onDone()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setPending(false)
    }
  }

  return (
    <div className="space-y-2">
      {isRoot ? (
        <Button
          type="button"
          size="sm"
          onClick={() => {
            setOpen(true)
          }}
        >
          <WrenchGlassIcon size="1.4em" />
          {t('doctorPage.fixAll', { count: safe.length })}
        </Button>
      ) : null}
      <FixCli
        steps={[
          {
            label: t('doctorPage.fixAllCli'),
            command: cliCommand(brand.BinaryName, 'doctor --fix'),
          },
        ]}
      />
      <ConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title={t('doctorPage.fixAllTitle', { count: safe.length })}
        description={t('doctorPage.fixAllBody')}
        consequences={consequences}
        confirmLabel={t('doctorPage.fixAllConfirm')}
        pendingLabel={t('doctorPage.fixAllPending')}
        pending={pending}
        error={error}
        onConfirm={() => void run()}
      />
    </div>
  )
}
