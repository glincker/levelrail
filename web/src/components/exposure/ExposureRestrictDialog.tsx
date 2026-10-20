import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Textarea } from '@/components/ui/textarea'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { toast } from '@/components/ui/toast'
import type { ExposureFinding, ExposurePlan } from '../../types/exposure'
import {
  useApplyRestriction,
  usePreviewRestriction,
} from '../../queries/exposure'

function parseSources(text: string): string[] {
  return text
    .split(/[\s,]+/)
    .map((s) => s.trim())
    .filter(Boolean)
}

/** ExposureRestrictDialog previews the exact rules and applies them only after an explicit confirmation of what gets dropped. */
export function ExposureRestrictDialog({
  finding,
  open,
  onOpenChange,
}: {
  finding: ExposureFinding
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('exposure')
  const [text, setText] = useState('')
  const [local, setLocal] = useState(false)
  const [plan, setPlan] = useState<ExposurePlan | null>(null)
  const [confirmed, setConfirmed] = useState(false)
  const preview = usePreviewRestriction()
  const apply = useApplyRestriction()

  const input = {
    port: finding.host_port,
    protocol: finding.protocol,
    allow: parseSources(text),
    local_containers: local,
  }
  const hasSources = input.allow.length > 0 || local

  function onPreview() {
    setConfirmed(false)
    preview.mutate(input, {
      onSuccess: setPlan,
      onError: (err) => {
        setPlan(null)
        toast.add({
          title: t('failedToast'),
          description: err.message,
          type: 'error',
        })
      },
    })
  }

  function onApply() {
    apply.mutate(input, {
      onSuccess: () => {
        toast.add({ title: t('appliedToast'), type: 'success' })
        onOpenChange(false)
      },
      onError: (err) => {
        toast.add({
          title: t('failedToast'),
          description: err.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {t('dialog.title', {
              port: `${finding.host_port}/${finding.protocol}`,
            })}
          </DialogTitle>
          <DialogDescription>{t('dialog.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3 text-sm">
          <label className="block space-y-1">
            <span className="font-medium">{t('dialog.sources')}</span>
            <Textarea
              value={text}
              placeholder="203.0.113.7&#10;10.0.0.0/8"
              onChange={(e) => {
                setText(e.target.value)
                setPlan(null)
              }}
              className="font-mono"
            />
            <span className="text-xs text-muted-foreground">
              {t('dialog.sourcesHint')}
            </span>
          </label>
          <label className="flex items-center gap-2">
            <Checkbox
              checked={local}
              onCheckedChange={(v) => {
                setLocal(v === true)
                setPlan(null)
              }}
            />
            <span>{t('dialog.localContainers')}</span>
          </label>
          {plan ? (
            <div className="space-y-2">
              <p className="font-medium">{t('dialog.previewTitle')}</p>
              <pre className="overflow-x-auto rounded-md bg-muted p-2 font-mono text-xs">
                {plan.commands.join('\n')}
              </pre>
              <Alert>
                <AlertTitle>{t('dialog.effect')}</AlertTitle>
                <AlertDescription>{plan.drops}</AlertDescription>
              </Alert>
              {plan.warnings.map((w) => (
                <Alert key={w} variant="destructive">
                  <AlertDescription>{w}</AlertDescription>
                </Alert>
              ))}
              <p className="text-xs text-muted-foreground">
                {t('dialog.persistence')}: {plan.persistence}
              </p>
              <label className="flex items-center gap-2">
                <Checkbox
                  checked={confirmed}
                  onCheckedChange={(v) => setConfirmed(v === true)}
                />
                <span>{t('dialog.confirm')}</span>
              </label>
            </div>
          ) : null}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('dialog.cancel')}
          </Button>
          <Button
            variant="outline"
            disabled={!hasSources || preview.isPending}
            onClick={onPreview}
          >
            {t('dialog.preview')}
          </Button>
          <Button
            disabled={!plan || !confirmed || apply.isPending}
            onClick={onApply}
          >
            {t('dialog.apply')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
