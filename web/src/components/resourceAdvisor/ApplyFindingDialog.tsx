import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'
import { useApplyFinding, useApplyPreview } from '../../queries/resourceAdvisor'
import { formatBytes, formatNanoCpus } from '../../lib/format'
import type {
  AdvisorApplyResult,
  AdvisorFinding,
} from '../../types/resourceAdvisor'

function useRow(preview: AdvisorApplyResult) {
  const { t } = useTranslation('resourceAdvisor')
  const notSet = t('apply.notSet')
  switch (preview.action.type) {
    case 'set_memory_limit':
      return {
        label: t('apply.memory'),
        before: preview.before.memory_bytes
          ? formatBytes(preview.before.memory_bytes)
          : notSet,
        after: formatBytes(preview.after.memory_bytes),
      }
    case 'set_cpu_limit':
      return {
        label: t('apply.cpu'),
        before: preview.before.nano_cpus
          ? formatNanoCpus(preview.before.nano_cpus)
          : notSet,
        after: formatNanoCpus(preview.after.nano_cpus),
      }
    default: {
      const sleep = (n?: number) =>
        n ? t('apply.sleepAfter', { count: n }) : t('apply.never')
      return {
        label: t('apply.sleep'),
        before: sleep(preview.before.sleep_idle_minutes),
        after: sleep(preview.after.sleep_idle_minutes),
      }
    }
  }
}

function PreviewTable({ preview }: { preview: AdvisorApplyResult }) {
  const { t } = useTranslation('resourceAdvisor')
  const row = useRow(preview)
  return (
    <table className="w-full text-sm">
      <thead>
        <tr className="text-left text-xs text-muted-foreground">
          <th className="pb-1 font-medium">{t('apply.setting')}</th>
          <th className="pb-1 font-medium">{t('apply.before')}</th>
          <th className="pb-1 font-medium">{t('apply.after')}</th>
        </tr>
      </thead>
      <tbody>
        <tr>
          <td className="py-1 text-muted-foreground">{row.label}</td>
          <td className="py-1 tabular-nums line-through decoration-muted-foreground/50">
            {row.before}
          </td>
          <td className="py-1 font-medium tabular-nums text-foreground">
            {row.after}
          </td>
        </tr>
      </tbody>
    </table>
  )
}

// Asks the server for a preview first and saves only on confirm, so the
// operator always sees the exact before and after.
export function ApplyFindingDialog({
  finding,
  open,
  onOpenChange,
}: {
  finding: AdvisorFinding
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('resourceAdvisor')
  const preview = useApplyPreview(finding.id, open)
  const apply = useApplyFinding()

  function confirm() {
    apply.mutate(finding.id, {
      onSuccess: (res) => {
        toast.add({
          type: 'success',
          title: res.applied_live
            ? t('apply.appliedLive', { name: res.resource.name })
            : t('apply.appliedSaved', { name: res.resource.name }),
        })
        onOpenChange(false)
      },
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('apply.title')}</DialogTitle>
          <DialogDescription>{t('apply.description')}</DialogDescription>
        </DialogHeader>
        <p className="text-sm font-medium text-foreground">{finding.title}</p>
        {preview.isPending ? (
          <p className="text-sm text-muted-foreground">{t('apply.loading')}</p>
        ) : preview.isError ? (
          <Alert variant="destructive">
            <AlertDescription>
              {t('apply.error', { message: preview.error.message })}
            </AlertDescription>
          </Alert>
        ) : (
          <PreviewTable preview={preview.data} />
        )}
        {finding.impact.risk ? (
          <p className="text-xs text-muted-foreground">{finding.impact.risk}</p>
        ) : null}
        {apply.isError ? (
          <Alert variant="destructive">
            <AlertDescription>
              {t('apply.failed', { message: apply.error.message })}
            </AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('apply.cancel')}
          </Button>
          <Button
            onClick={confirm}
            disabled={!preview.isSuccess || apply.isPending}
          >
            {apply.isPending ? t('apply.pending') : t('apply.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
