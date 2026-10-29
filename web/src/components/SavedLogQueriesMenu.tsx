import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  BookmarkGlassIcon,
  PinGlassIcon,
  RestoreGlassIcon,
  TrashGlassIcon,
  UsersGlassIcon,
} from '@thesvg/ui-react'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Popover, PopoverContent, PopoverTrigger } from './ui/popover'
import { toast } from './ui/toast'
import {
  useDeleteLogQuery,
  useSavedLogQueries,
  useSaveLogQuery,
} from '../queries/logExplorer'
import {
  describeQuery,
  isEmptyQuery,
  readRecentQueries,
  type RecentQuery,
} from '../lib/logExplorer'
import type { LogQuerySpec, SavedLogQuery } from '../types/logExplorer'

// Saved and recent log searches for one app. Saved queries live on the
// server (private, or shared with everyone who can see the app, optionally
// pinned to this app); recent ones are a per-browser convenience.
export function SavedLogQueriesMenu({
  appName,
  current,
  currentSpec,
  onApply,
}: {
  appName: string
  current: RecentQuery
  currentSpec: LogQuerySpec
  onApply: (spec: LogQuerySpec) => void
}) {
  const { t } = useTranslation('observability')
  const saved = useSavedLogQueries(appName)
  const save = useSaveLogQuery(appName)
  const remove = useDeleteLogQuery(appName)
  const [name, setName] = useState('')
  const [share, setShare] = useState(false)
  const [pin, setPin] = useState(false)

  const recent = readRecentQueries(appName)
  const queries = saved.data ?? []
  const canSave = name.trim() !== '' && !isEmptyQuery(current)

  function apply(q: SavedLogQuery) {
    onApply(q.spec)
  }

  return (
    <Popover>
      <PopoverTrigger
        render={<Button type="button" size="sm" variant="outline" />}
      >
        <BookmarkGlassIcon className="size-4" aria-hidden="true" />
        {t('explorer.saved')}
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 max-w-[calc(100vw-2rem)]">
        <div className="flex flex-col gap-2">
          <p className="px-1 text-xs font-medium text-muted-foreground">
            {t('explorer.savedHeading')}
          </p>
          {queries.length === 0 ? (
            <p className="px-1 text-xs text-muted-foreground">
              {t('explorer.savedEmpty')}
            </p>
          ) : null}
          {queries.map((q) => (
            <div key={q.id} className="flex items-center gap-1">
              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="min-w-0 flex-1 justify-start"
                onClick={() => {
                  apply(q)
                }}
              >
                {q.pinned ? (
                  <PinGlassIcon
                    className="size-4 shrink-0"
                    aria-hidden="true"
                  />
                ) : null}
                {q.visibility === 'instance' ? (
                  <UsersGlassIcon
                    className="size-4 shrink-0"
                    aria-hidden="true"
                  />
                ) : null}
                <span className="truncate">{q.name}</span>
              </Button>
              {q.mine ? (
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  aria-label={t('explorer.deleteSaved', { name: q.name })}
                  disabled={remove.isPending}
                  onClick={() => {
                    remove.mutate(q.id)
                  }}
                >
                  <TrashGlassIcon className="size-4" aria-hidden="true" />
                </Button>
              ) : null}
            </div>
          ))}

          {recent.length > 0 ? (
            <>
              <p className="mt-1 flex items-center gap-1 px-1 text-xs font-medium text-muted-foreground">
                <RestoreGlassIcon className="size-4" aria-hidden="true" />
                {t('explorer.recent')}
              </p>
              {recent.map((r) => (
                <Button
                  key={describeQuery(r)}
                  type="button"
                  size="sm"
                  variant="ghost"
                  className="justify-start font-mono text-xs"
                  onClick={() => {
                    onApply({ q: r.q, level: r.level, fields: r.field })
                  }}
                >
                  <span className="truncate">{describeQuery(r)}</span>
                </Button>
              ))}
            </>
          ) : null}

          <form
            className="mt-1 flex flex-col gap-2 border-t border-border pt-2"
            onSubmit={(e) => {
              e.preventDefault()
              if (!canSave) {
                return
              }
              save.mutate(
                {
                  name: name.trim(),
                  app: appName,
                  spec: currentSpec,
                  visibility: share ? 'instance' : 'private',
                  pinned: pin,
                },
                {
                  onSuccess: () => {
                    setName('')
                    toast.add({
                      title: t('explorer.savedToast'),
                      type: 'success',
                    })
                  },
                },
              )
            }}
          >
            <Input
              value={name}
              onChange={(e) => {
                setName(e.target.value)
              }}
              placeholder={t('explorer.saveName')}
              aria-label={t('explorer.saveName')}
            />
            <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <input
                type="checkbox"
                checked={share}
                onChange={(e) => {
                  setShare(e.target.checked)
                }}
              />
              {t('explorer.shareWithTeam')}
            </label>
            <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <input
                type="checkbox"
                checked={pin}
                onChange={(e) => {
                  setPin(e.target.checked)
                }}
              />
              {t('explorer.pinToApp')}
            </label>
            <Button
              type="submit"
              size="sm"
              disabled={!canSave || save.isPending}
            >
              {t('explorer.saveCurrent')}
            </Button>
            {save.error ? (
              <p className="text-xs text-destructive">{save.error.message}</p>
            ) : null}
            {isEmptyQuery(current) ? (
              <p className="text-xs text-muted-foreground">
                {t('explorer.saveNeedsFilter')}
              </p>
            ) : null}
          </form>
        </div>
      </PopoverContent>
    </Popover>
  )
}
