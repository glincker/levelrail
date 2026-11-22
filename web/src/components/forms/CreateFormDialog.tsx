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
import { useNotificationChannelsOptional } from '../../queries/notificationChannels'
import {
  FORM_RECIPES,
  recipeChannelIds,
  type FormRecipe,
} from '../../lib/formRecipes'

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
  const { create, saveFields, save } = useFormActions(app)
  const channels = useNotificationChannelsOptional()
  const [recipe, setRecipe] = useState<FormRecipe | null>(null)
  const [name, setName] = useState('')
  const valid = NAME_PATTERN.test(name)

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!valid) return
    create.mutate(name, {
      onSuccess: (created) => {
        // Recipe fields and notify channel are best effort: the form exists
        // either way and both can be edited on its card.
        if (recipe) {
          saveFields.mutate({ form: created.name, schema: recipe.schema })
          const ids = recipeChannelIds(recipe, channels.data ?? [])
          if (ids.length > 0) {
            save.mutate({
              form: created.name,
              input: {
                redirect_url: created.redirect_url,
                honeypot_field: created.honeypot_field,
                notify_channel_ids: ids,
              },
            })
          }
        }
        setName('')
        setRecipe(null)
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
            <Label>{t('recipes.title')}</Label>
            <div className="grid gap-2 sm:grid-cols-3">
              {FORM_RECIPES.map((r) => (
                <button
                  key={r.id}
                  type="button"
                  aria-pressed={recipe?.id === r.id}
                  onClick={() => {
                    const next = recipe?.id === r.id ? null : r
                    setRecipe(next)
                    if (next && !name) setName(next.name)
                  }}
                  className="rounded-lg border border-border p-2 text-left text-sm hover:bg-muted aria-pressed:border-ring aria-pressed:bg-muted"
                >
                  <span className="block font-medium">
                    {t(`recipes.${r.id}.title`)}
                  </span>
                  <span className="block text-xs text-muted-foreground">
                    {t(`recipes.${r.id}.body`)}
                  </span>
                </button>
              ))}
            </div>
          </div>
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
