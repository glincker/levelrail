import { useState, type ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
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
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useSaveEnvironment } from '../../queries/globalEnvironments'
import { isBuiltIn } from '../../lib/environmentBuiltIn'
import type {
  EnvironmentKind,
  GlobalEnvironment,
} from '../../types/environment'

const ASSIGNABLE_KINDS = ['dev', 'test', 'uat', 'production', 'custom'] as const

export function EnvironmentFormDialog({
  trigger,
  environment,
}: {
  trigger: ReactElement
  environment?: GlobalEnvironment
}) {
  const { t } = useTranslation('environments')
  const [open, setOpen] = useState(false)
  const [name, setName] = useState(environment?.name ?? '')
  const [kind, setKind] = useState<EnvironmentKind>(
    environment?.kind ?? 'custom',
  )
  const [isProtected, setProtected] = useState(environment?.protected ?? false)
  const save = useSaveEnvironment()
  const kindLocked = environment?.kind === 'preview' || isBuiltIn(environment)

  function submit() {
    save.mutate(
      { id: environment?.id, name: name.trim(), kind, protected: isProtected },
      {
        onSuccess: (saved) => {
          setOpen(false)
          toast.add({
            title: t(environment ? 'form.updated' : 'form.created', {
              name: saved.name,
            }),
            type: 'success',
          })
        },
        onError: (error) => {
          toast.add({
            title: t('form.failed'),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={trigger} />
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {t(environment ? 'form.editTitle' : 'form.createTitle')}
          </DialogTitle>
          <DialogDescription>{t('form.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-1.5">
            <label
              htmlFor="environment-name"
              className="text-sm font-medium text-foreground"
            >
              {t('form.name')}
            </label>
            <Input
              id="environment-name"
              value={name}
              placeholder={t('form.namePlaceholder')}
              onChange={(e) => {
                setName(e.target.value)
              }}
            />
          </div>
          <div className="space-y-1.5">
            <label
              htmlFor="environment-kind"
              className="text-sm font-medium text-foreground"
            >
              {t('form.kind')}
            </label>
            <Select
              value={kind}
              disabled={kindLocked}
              onValueChange={(next) => {
                if (next) {
                  setKind(next)
                }
              }}
            >
              <SelectTrigger id="environment-kind" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {kindLocked && !ASSIGNABLE_KINDS.some((k) => k === kind) ? (
                  <SelectItem value={kind}>{t(`kind.${kind}`)}</SelectItem>
                ) : null}
                {ASSIGNABLE_KINDS.map((k) => (
                  <SelectItem key={k} value={k}>
                    {t(`kind.${k}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <label className="flex items-start gap-3 text-sm">
            <Switch
              checked={isProtected}
              aria-label={t('form.protected')}
              onCheckedChange={setProtected}
            />
            <span className="space-y-0.5">
              <span className="block font-medium text-foreground">
                {t('form.protected')}
              </span>
              <span className="block text-muted-foreground">
                {t('form.protectedHint')}
              </span>
            </span>
          </label>
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setOpen(false)
            }}
          >
            {t('form.cancel')}
          </Button>
          <Button
            type="button"
            disabled={save.isPending || name.trim() === ''}
            onClick={submit}
          >
            {save.isPending ? t('form.saving') : t('form.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
