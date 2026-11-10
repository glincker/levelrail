import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ImportGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { importBundle, type BundleImportResult } from '../../queries/appBundle'

// Import is always previewed first: the plan comes from a dry run, and only
// the reviewed file is sent again. Services land stopped.
export default function ImportBundleDialog() {
  const { t } = useTranslation('compose')
  const [open, setOpen] = useState(false)
  const [file, setFile] = useState<File | null>(null)
  const [name, setName] = useState('')
  const [plan, setPlan] = useState<BundleImportResult | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  function reset() {
    setFile(null)
    setPlan(null)
    setError('')
  }

  async function run(dryRun: boolean) {
    if (!file) return
    setBusy(true)
    setError('')
    try {
      const res = await importBundle(file, name.trim(), dryRun)
      if (dryRun) {
        setPlan(res)
      } else {
        toast.add({
          title: t('bundleImport.done', { app: res.app }),
          type: 'success',
        })
        setOpen(false)
        reset()
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) reset()
      }}
    >
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <ImportGlassIcon size="1.4em" aria-hidden="true" />
        {t('bundleImport.open')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('bundleImport.title')}</DialogTitle>
          <DialogDescription>{t('bundleImport.description')}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <Input
            type="file"
            accept=".tar,application/x-tar"
            aria-label={t('bundleImport.file')}
            onChange={(e) => {
              setFile(e.target.files?.[0] ?? null)
              setPlan(null)
            }}
          />
          <Input
            value={name}
            placeholder={t('bundleImport.namePlaceholder')}
            aria-label={t('bundleImport.name')}
            onChange={(e) => {
              setName(e.target.value)
              setPlan(null)
            }}
          />
          {plan ? (
            <div className="flex flex-col gap-1 text-sm">
              <p className="font-medium">
                {t('bundleImport.plan', {
                  app: plan.app,
                  count: plan.services.length,
                })}
              </p>
              <ul className="text-muted-foreground">
                {plan.services.map((s) => (
                  <li key={s.name} className="truncate">
                    {s.name} {s.image}
                  </li>
                ))}
              </ul>
              {plan.secrets_to_set?.length ? (
                <p className="text-muted-foreground">
                  {t('bundleImport.secrets', {
                    names: plan.secrets_to_set.join(', '),
                  })}
                </p>
              ) : null}
              {plan.warnings?.map((w) => (
                <p key={w} className="text-warning">
                  {w}
                </p>
              ))}
            </div>
          ) : null}
          {error ? (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          {plan ? (
            <Button disabled={busy} onClick={() => void run(false)}>
              {t('bundleImport.confirm')}
            </Button>
          ) : (
            <Button disabled={busy || !file} onClick={() => void run(true)}>
              {t('bundleImport.preview')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
