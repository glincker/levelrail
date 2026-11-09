import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { BookmarkGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { toast } from '@/components/ui/toast'
import { ApiError, readErrorMessage } from '../lib/apiError'
import {
  sanitizeViewFilter,
  type SavedView,
  type SavedViewList,
  type ViewFilter,
} from '../lib/savedViews'

async function viewsRequest<T>(
  url: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

const viewsKey = (list: SavedViewList) => ['saved-views', list] as const

export function SavedViewsMenu({
  list,
  current,
  onApply,
}: {
  list: SavedViewList
  current: ViewFilter
  onApply: (filter: ViewFilter) => void
}) {
  const { t } = useTranslation('uiShared')
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [team, setTeam] = useState(false)
  const views = useQuery({
    queryKey: viewsKey(list),
    queryFn: ({ signal }) =>
      viewsRequest<{ views: SavedView[] }>(
        `/api/v1/saved-views?list=${list}`,
        { signal },
        'list saved views',
      ).then((r) => r.views),
    staleTime: 30_000,
  })
  const invalidate = () => qc.invalidateQueries({ queryKey: viewsKey(list) })
  const save = useMutation({
    mutationFn: (body: {
      name: string
      list: SavedViewList
      filter: ViewFilter
      shared: 'private' | 'team'
    }) =>
      viewsRequest<SavedView>(
        '/api/v1/saved-views',
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        },
        'save view',
      ),
    onSuccess: invalidate,
  })
  const remove = useMutation({
    mutationFn: (id: string) =>
      viewsRequest<undefined>(
        `/api/v1/saved-views/${encodeURIComponent(id)}`,
        { method: 'DELETE' },
        'delete view',
      ),
    onSuccess: invalidate,
  })

  const saveCurrent = () => {
    const trimmed = name.trim()
    if (!trimmed) return
    save.mutate(
      {
        name: trimmed,
        list,
        filter: sanitizeViewFilter(list, current),
        shared: team ? 'team' : 'private',
      },
      {
        onSuccess: () => {
          setName('')
          toast.add({
            title: t('savedViews.saved', { name: trimmed }),
            type: 'success',
          })
        },
        onError: (err) => toast.add({ title: err.message, type: 'error' }),
      },
    )
  }

  const items = views.data ?? []
  return (
    <Popover>
      <PopoverTrigger
        render={<Button type="button" size="sm" variant="outline" />}
      >
        <BookmarkGlassIcon size="1.4em" />
        {t('savedViews.button')}
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72 space-y-3">
        {items.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('savedViews.empty')}
          </p>
        ) : (
          <ul aria-label={t('savedViews.label')} className="space-y-1">
            {items.map((v) => (
              <li key={v.id} className="flex items-center gap-1">
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  className="min-w-0 flex-1 justify-start"
                  aria-label={t('savedViews.apply', { name: v.name })}
                  onClick={() => onApply(sanitizeViewFilter(list, v.filter))}
                >
                  <span className="truncate">{v.name}</span>
                  {v.shared === 'team' ? (
                    <span className="ml-auto shrink-0 text-xs text-muted-foreground">
                      {t('savedViews.teamBadge')}
                    </span>
                  ) : null}
                </Button>
                {v.can_delete ? (
                  <Button
                    type="button"
                    size="icon-sm"
                    variant="ghost"
                    aria-label={t('savedViews.delete', { name: v.name })}
                    onClick={() =>
                      remove.mutate(v.id, {
                        onSuccess: () =>
                          toast.add({
                            title: t('savedViews.deleted'),
                            type: 'success',
                          }),
                        onError: (err) =>
                          toast.add({ title: err.message, type: 'error' }),
                      })
                    }
                  >
                    <TrashGlassIcon size="1.4em" />
                  </Button>
                ) : null}
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
          <Input
            value={name}
            maxLength={64}
            aria-label={t('savedViews.namePlaceholder')}
            placeholder={t('savedViews.namePlaceholder')}
            onChange={(e) => setName(e.target.value)}
          />
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              className="size-4 accent-primary"
              checked={team}
              onChange={(e) => setTeam(e.target.checked)}
            />
            {t('savedViews.shareTeam')}
          </label>
          <Button
            type="submit"
            size="sm"
            disabled={!name.trim() || save.isPending}
          >
            {t('savedViews.save')}
          </Button>
        </form>
      </PopoverContent>
    </Popover>
  )
}

export default SavedViewsMenu
