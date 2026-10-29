import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { BookmarkGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { useSaveSearch } from '../queries/search'

export function SaveSearchButton({ onToggle }: { onToggle: () => void }) {
  const { t } = useTranslation('palette')
  return (
    <button
      type="button"
      onClick={onToggle}
      className="flex items-center gap-1 rounded px-1 text-xs text-muted-foreground hover:text-foreground"
    >
      <BookmarkGlassIcon className="size-4" aria-hidden="true" />
      {t('save.button')}
    </button>
  )
}

export function SaveSearchForm({
  query,
  onDone,
}: {
  query: string
  onDone: () => void
}) {
  const { t } = useTranslation('palette')
  const [name, setName] = React.useState(query)
  const save = useSaveSearch()

  function submit(e: React.FormEvent) {
    e.preventDefault()
    const trimmed = name.trim()
    if (trimmed === '') return
    save.mutate(
      { name: trimmed, query },
      {
        onSuccess: () => {
          toast.add({
            title: t('save.saved', { name: trimmed }),
            type: 'success',
          })
          onDone()
        },
        onError: (error) =>
          toast.add({
            title: t('save.failed', { message: error.message }),
            type: 'error',
          }),
      },
    )
  }

  return (
    <form
      onSubmit={submit}
      className="flex items-center gap-2 border-t border-border px-3 py-2"
    >
      <Input
        autoFocus
        value={name}
        onChange={(e) => setName(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            e.preventDefault()
            e.stopPropagation()
            onDone()
          }
        }}
        aria-label={t('save.nameLabel')}
        className="h-8"
      />
      <Button type="submit" size="sm" disabled={save.isPending}>
        {t('save.submit')}
      </Button>
    </form>
  )
}
