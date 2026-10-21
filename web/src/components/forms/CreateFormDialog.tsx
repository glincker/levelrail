import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useFormActions } from '../../queries/forms'

const NAME_PATTERN = /^[a-z0-9][a-z0-9_-]{0,62}$/

export function CreateFormDialog({
  app,
  open,
  onOpenChange,
}: {
  app: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('forms')
  const { create } = useFormActions(app)
  const [name, setName] = useState('')
  const valid = NAME_PATTERN.test(name)

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!valid) return
    create.mutate(name, {
      onSuccess: () => {
        setName('')
        onOpenChange(false)
      },
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{t('create.title')}</DialogTitle>
            <DialogDescription>{t('create.description')}</DialogDescription>
          </DialogHeader>
          <div className="space-y-1.5">
            <Label htmlFor="form-name">{t('create.name')}</Label>
            <Input
              id="form-name"
              value={name}
              onChange={(e) => setName(e.target.value.toLowerCase())}
              placeholder={t('create.namePlaceholder')}
              aria-invalid={name !== '' && !valid}
              autoComplete="off"
              autoFocus
            />
            {create.isError && (
              <p role="alert" className="text-sm text-destructive">
                {create.error.message}
              </p>
            )}
          </div>
          <DialogFooter>
            <Button type="submit" disabled={!valid || create.isPending}>
              {create.isPending ? t('create.submitting') : t('create.submit')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
