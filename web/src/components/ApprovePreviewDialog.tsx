import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ShieldWarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { toast } from '@/components/ui/toast'
import { useApprovePreviewEnvironment } from '../queries/previewPolicy'
import type { PreviewEnvironment } from '../types/previewEnvironment'

// ApprovePreviewDialog deploys a held fork pull request once. A fork gets no
// environment variables or secrets unless the reviewer opts in here.
export function ApprovePreviewDialog({
  appName,
  preview,
}: {
  appName: string
  preview: PreviewEnvironment
}) {
  const { t } = useTranslation('previews')
  const [open, setOpen] = useState(false)
  const [shareSecrets, setShareSecrets] = useState(false)
  const approve = useApprovePreviewEnvironment(appName)

  function confirm() {
    approve.mutate(
      { prNumber: preview.pr_number, shareSecrets },
      {
        onSuccess: () => {
          setOpen(false)
          toast.add({
            title: t('approve.deploying', { number: preview.pr_number }),
            type: 'success',
          })
        },
        onError: (error) => {
          toast.add({
            title: t('approve.error'),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button type="button" size="sm" />}>
        {t('approve.button')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {t('approve.title', { number: preview.pr_number })}
          </DialogTitle>
          <DialogDescription>
            {t('approve.fromRepo', {
              repo: preview.head_repo ?? t('approve.anotherRepo'),
            })}
          </DialogDescription>
        </DialogHeader>
        <Alert variant="destructive">
          <ShieldWarningIcon aria-hidden="true" />
          <AlertDescription>{t('approve.warning')}</AlertDescription>
        </Alert>
        <div className="flex items-start gap-2">
          <Checkbox
            id="approve-share-secrets"
            checked={shareSecrets}
            onCheckedChange={(next) => {
              setShareSecrets(next === true)
            }}
          />
          <div className="space-y-0.5">
            <Label htmlFor="approve-share-secrets">
              {t('approve.shareSecrets')}
            </Label>
            <p className="text-xs text-muted-foreground">
              {t('approve.shareSecretsHint')}
            </p>
          </div>
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setOpen(false)
            }}
          >
            {t('approve.cancel')}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={approve.isPending}
            onClick={confirm}
          >
            {t('approve.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
