import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { useAttachAppTag, useTags } from '../queries/tags'

/** AddTagDialog is opened from the overview overflow menu so the header carries no lone add button. */
export function AddTagDialog({
  appName,
  open,
  onOpenChange,
}: {
  appName: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('common')
  const [value, setValue] = useState('')
  const { data: allTags } = useTags()
  const attachTag = useAttachAppTag(appName)

  const add = () => {
    const name = value.trim()
    if (!name) return
    attachTag.mutate(name, {
      onSuccess: () => {
        setValue('')
        onOpenChange(false)
      },
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{t('overviewHero.addTagTitle')}</DialogTitle>
        </DialogHeader>
        <form
          className="flex items-center gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            add()
          }}
        >
          <Input
            autoFocus
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder={t('overviewHero.tagName')}
            list="overview-tag-suggestions"
            aria-label={t('overviewHero.tagName')}
            className="h-8 text-sm"
          />
          <datalist id="overview-tag-suggestions">
            {(allTags ?? []).map((tag) => (
              <option key={tag.id} value={tag.name} />
            ))}
          </datalist>
          <Button
            type="submit"
            size="sm"
            disabled={!value.trim() || attachTag.isPending}
          >
            {attachTag.isPending
              ? t('overviewHero.tagAdding')
              : t('overviewHero.tagAdd')}
          </Button>
        </form>
        {attachTag.isError ? (
          <p className="text-xs text-destructive">{attachTag.error.message}</p>
        ) : null}
        <DialogFooter className="sr-only" />
      </DialogContent>
    </Dialog>
  )
}
