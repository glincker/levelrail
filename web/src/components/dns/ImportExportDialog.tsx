import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { DownloadSimpleIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { exportDnsZoneUrl, useImportDnsRecords } from '../../queries/dns'
import type { DnsPlanResult } from '../../types/dns'
import { PlanView } from './PlanView'

function guessFormat(text: string): 'json' | 'bind' {
  const s = text.trimStart()
  return s.startsWith('{') || s.startsWith('[') ? 'json' : 'bind'
}

/** ImportExportDialog previews a zone file or JSON import as a plan before applying, and downloads exports. */
export function ImportExportDialog({
  zone,
  zoneName,
  onClose,
}: {
  zone: string
  zoneName: string
  onClose: () => void
}) {
  const { t } = useTranslation('dns')
  const importer = useImportDnsRecords(zone)
  const [content, setContent] = useState('')
  const [replace, setReplace] = useState(false)
  const [confirm, setConfirm] = useState('')
  const [plan, setPlan] = useState<DnsPlanResult | null>(null)

  function run(apply: boolean) {
    importer.mutate(
      { format: guessFormat(content), content, replace, apply, confirm },
      {
        onSuccess: (res) => {
          setPlan(res)
          if (apply) {
            toast.add({
              title: t('import.appliedToast', { count: res.applied }),
              type: 'success',
            })
            onClose()
          }
        },
        onError: (err) =>
          toast.add({
            title: t('import.failedToast'),
            description: err.message,
            type: 'error',
          }),
      },
    )
  }

  async function loadFile(file: File | undefined) {
    if (!file) return
    setContent(await file.text())
    setPlan(null)
  }

  const canApply =
    plan !== null &&
    !plan.plan.blocked &&
    (plan.errors ?? []).length === 0 &&
    (!replace || confirm === zoneName)

  return (
    <Dialog open onOpenChange={(o) => (o ? undefined : onClose())}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('import.title')}</DialogTitle>
          <DialogDescription>
            {t('import.lead', { zone: zoneName })}
          </DialogDescription>
        </DialogHeader>
        <Tabs defaultValue="import">
          <TabsList>
            <TabsTrigger value="import">{t('import.tabImport')}</TabsTrigger>
            <TabsTrigger value="export">{t('import.tabExport')}</TabsTrigger>
          </TabsList>
          <TabsContent value="import" className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="dns-import-file">{t('import.file')}</Label>
              <Input
                id="dns-import-file"
                type="file"
                accept=".zone,.txt,.json,.db"
                onChange={(e) => void loadFile(e.target.files?.[0])}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="dns-import-content">{t('import.paste')}</Label>
              <Textarea
                id="dns-import-content"
                rows={6}
                className="font-mono text-xs"
                value={content}
                placeholder={
                  'www 300 IN A 203.0.113.10\n@ 3600 IN MX 10 mail.example.com.'
                }
                onChange={(e) => {
                  setContent(e.target.value)
                  setPlan(null)
                }}
              />
              <p className="text-xs text-muted-foreground">
                {t('import.formatHint')}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                id="dns-import-replace"
                checked={replace}
                onCheckedChange={(v) => {
                  setReplace(v === true)
                  setPlan(null)
                }}
              />
              <Label htmlFor="dns-import-replace">{t('import.replace')}</Label>
            </div>
            {replace ? (
              <div className="space-y-1.5">
                <Label htmlFor="dns-import-confirm">
                  {t('import.confirm', { zone: zoneName })}
                </Label>
                <Input
                  id="dns-import-confirm"
                  className="font-mono"
                  value={confirm}
                  onChange={(e) => setConfirm(e.target.value)}
                />
              </div>
            ) : null}
            {plan ? <PlanView result={plan} /> : null}
            <div className="flex justify-end gap-2">
              <Button
                variant="outline"
                disabled={content.trim() === '' || importer.isPending}
                onClick={() => run(false)}
              >
                {t('import.preview')}
              </Button>
              <Button
                disabled={!canApply || importer.isPending}
                onClick={() => run(true)}
              >
                {t('import.apply')}
              </Button>
            </div>
          </TabsContent>
          <TabsContent value="export" className="space-y-3">
            <p className="text-sm text-muted-foreground">
              {t('import.exportLead')}
            </p>
            <div className="flex gap-2">
              <Button
                variant="outline"
                render={<a href={exportDnsZoneUrl(zone, 'bind')} download />}
                nativeButton={false}
              >
                <DownloadSimpleIcon />
                {t('import.exportBind')}
              </Button>
              <Button
                variant="outline"
                render={<a href={exportDnsZoneUrl(zone, 'json')} download />}
                nativeButton={false}
              >
                <DownloadSimpleIcon />
                {t('import.exportJson')}
              </Button>
            </div>
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  )
}
