import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import type { GrantResult, GrantTemplate } from '../../types/databaseAccess'
import { useGrantAccess } from '../../queries/databaseAccess'
import { tokenListQueryOptions } from '../../queries/tokens'
import { userListQueryOptions } from '../../queries/users'

type PrincipalType = 'user' | 'token'

/** GrantDialog applies a database scoped IAM template to a user or token, previewing the exact policy first. */
export function GrantDialog({
  databaseName,
  open,
  onOpenChange,
  templates,
}: {
  databaseName: string
  open: boolean
  onOpenChange: (open: boolean) => void
  templates: GrantTemplate[]
}) {
  const { t } = useTranslation('databaseAccess')
  const grant = useGrantAccess(databaseName)
  const users = useQuery({ ...userListQueryOptions(), enabled: open })
  const tokens = useQuery({ ...tokenListQueryOptions(), enabled: open })
  const [type, setType] = useState<PrincipalType>('user')
  const [principal, setPrincipal] = useState('')
  const [template, setTemplate] = useState<GrantTemplate>('database-read-only')
  const [preview, setPreview] = useState<GrantResult | null>(null)

  const options =
    type === 'user'
      ? (users.data ?? []).map((u) => ({
          id: u.id,
          label: u.display_name || u.email,
        }))
      : (tokens.data ?? [])
          .filter((tok) => !tok.revoked_at)
          .map((tok) => ({ id: tok.id, label: tok.name }))

  function run(previewOnly: boolean) {
    grant.mutate(
      {
        template,
        principal_type: type,
        principal_id: principal,
        preview: previewOnly,
      },
      {
        onSuccess: (res) => {
          if (previewOnly) {
            setPreview(res)
            return
          }
          toast.add({ title: t('grant.appliedToast'), type: 'success' })
          setPreview(null)
          onOpenChange(false)
        },
        onError: (err) =>
          toast.add({
            title: t('grant.failedToast'),
            description: err.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('grant.title')}</DialogTitle>
          <DialogDescription>{t('grant.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label>{t('grant.principalType')}</Label>
              <Select
                value={type}
                onValueChange={(v) => {
                  setType(v as PrincipalType)
                  setPrincipal('')
                  setPreview(null)
                }}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="user">{t('grant.user')}</SelectItem>
                  <SelectItem value="token">{t('grant.token')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label>{t('grant.principal')}</Label>
              <Select
                value={principal}
                onValueChange={(v) => {
                  setPrincipal(v ?? '')
                  setPreview(null)
                }}
              >
                <SelectTrigger className="w-full">
                  <SelectValue placeholder={t('grant.principal')} />
                </SelectTrigger>
                <SelectContent>
                  {options.map((o) => (
                    <SelectItem key={o.id} value={o.id}>
                      {o.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="space-y-1.5">
            <Label>{t('grant.template')}</Label>
            <Select
              value={template}
              onValueChange={(v) => {
                setTemplate(v as GrantTemplate)
                setPreview(null)
              }}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {templates.map((tpl) => (
                  <SelectItem key={tpl} value={tpl}>
                    {t(`grant.templates.${tpl}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              {t(`grant.templateHint.${template}`)}
            </p>
          </div>
          {preview ? (
            <div className="space-y-1">
              <p className="text-xs font-medium">{t('grant.previewTitle')}</p>
              <pre className="max-h-48 overflow-auto rounded-md bg-muted p-2 font-mono text-xs">
                {JSON.stringify(preview.document, null, 2)}
              </pre>
              {preview.notes.map((n) => (
                <p key={n} className="text-xs text-muted-foreground">
                  {n}
                </p>
              ))}
            </div>
          ) : null}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('users.cancel')}
          </Button>
          <Button
            variant="outline"
            disabled={!principal || grant.isPending}
            onClick={() => run(true)}
          >
            {t('grant.preview')}
          </Button>
          <Button
            disabled={!principal || !preview || grant.isPending}
            onClick={() => run(false)}
          >
            {t('grant.apply')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
