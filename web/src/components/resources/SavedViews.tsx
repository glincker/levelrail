import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BookmarkGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { toast } from '@/components/ui/toast'
import { parseView, serializeView } from '@/lib/resourceLinks'
import {
  useConsoleViews,
  useDeleteConsoleView,
  useSaveConsoleView,
  type ConsoleQuery,
} from '@/queries/resources'

export function SavedViews({
  current,
  onApply,
}: {
  current: ConsoleQuery
  onApply: (q: ConsoleQuery) => void
}) {
  const { t } = useTranslation('resources')
  const { data: views } = useConsoleViews()
  const save = useSaveConsoleView()
  const remove = useDeleteConsoleView()
  const [name, setName] = useState('')

  function saveCurrent() {
    const trimmed = name.trim()
    if (!trimmed) return
    save.mutate(
      { name: trimmed, query: serializeView(current) },
      {
        onSuccess: () => {
          setName('')
          toast.add({
            title: t('views.saved', { name: trimmed }),
            type: 'success',
          })
        },
        onError: (err) => toast.add({ title: err.message, type: 'error' }),
      },
    )
  }

  return (
    <Popover>
      <PopoverTrigger
        render={<Button type="button" size="sm" variant="outline" />}
      >
        <BookmarkGlassIcon size="1.2em" />
        {t('views.button')}
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72 space-y-3">
        {(views ?? []).length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('views.empty')}</p>
        ) : (
          <ul aria-label={t('views.label')} className="space-y-1">
            {(views ?? []).map((v) => (
              <li key={v.id} className="flex items-center gap-1">
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  className="min-w-0 flex-1 justify-start"
                  aria-label={t('views.apply', { name: v.name })}
                  onClick={() => onApply(parseView(v.query))}
                >
                  <span className="truncate">{v.name}</span>
                </Button>
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  aria-label={t('views.delete', { name: v.name })}
                  onClick={() =>
                    remove.mutate(v.id, {
                      onSuccess: () =>
                        toast.add({
                          title: t('views.deleted'),
                          type: 'success',
                        }),
                    })
                  }
                >
                  <TrashGlassIcon size="1.2em" />
                </Button>
              </li>
            ))}
          </ul>
        )}
        <form
          className="space-y-2 border-t border-border pt-3"
          onSubmit={(e) => {
            e.preventDefault()
            saveCurrent()
          }}
        >
          <p className="text-xs font-medium text-muted-foreground">
            {t('views.saveTitle')}
          </p>
          <Input
            value={name}
            maxLength={64}
            aria-label={t('views.namePlaceholder')}
            placeholder={t('views.namePlaceholder')}
            onChange={(e) => setName(e.target.value)}
          />
          <Button
            type="submit"
            size="sm"
            disabled={!name.trim() || save.isPending}
          >
            {t('views.save')}
          </Button>
        </form>
      </PopoverContent>
    </Popover>
  )
}
