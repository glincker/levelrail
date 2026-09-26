import { useState } from 'react'
import { PlusIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { useAttachAppTag, useDetachAppTag, useTags } from '../../queries/tags'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'

export function TagChips({
  appName,
  tags,
}: {
  appName: string
  tags?: string[]
}) {
  const [value, setValue] = useState('')
  const [open, setOpen] = useState(false)
  const { data: allTags } = useTags()
  const attachTag = useAttachAppTag(appName)
  const detachTag = useDetachAppTag(appName)
  const idByName = new Map((allTags ?? []).map((t) => [t.name, t.id]))

  const add = () => {
    const name = value.trim()
    if (!name) return
    attachTag.mutate(name, {
      onSuccess: () => {
        setValue('')
        setOpen(false)
      },
    })
  }

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {(tags ?? []).map((name) => {
        const id = idByName.get(name)
        return (
          <Badge key={name} variant="outline" className="gap-1 pr-1">
            {name}
            <button
              type="button"
              className="rounded-sm text-muted-foreground hover:text-foreground disabled:opacity-50"
              disabled={!id || detachTag.isPending}
              onClick={() => {
                if (id) detachTag.mutate(id)
              }}
            >
              <XIcon className="size-3" aria-hidden="true" />
              <span className="sr-only">Remove tag {name}</span>
            </button>
          </Badge>
        )
      })}
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          render={<Button variant="ghost" size="icon-xs" />}
          aria-label="Add tag"
        >
          <PlusIcon className="size-3" aria-hidden="true" />
        </PopoverTrigger>
        <PopoverContent align="start" className="w-56 gap-2">
          <Input
            autoFocus
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                add()
              }
            }}
            placeholder="Tag name"
            list="overview-tag-suggestions"
            aria-label="New tag name"
            className="h-8 text-sm"
          />
          <datalist id="overview-tag-suggestions">
            {(allTags ?? []).map((t) => (
              <option key={t.id} value={t.name} />
            ))}
          </datalist>
          <Button
            size="sm"
            disabled={!value.trim() || attachTag.isPending}
            onClick={add}
          >
            {attachTag.isPending ? 'Adding...' : 'Add tag'}
          </Button>
          {attachTag.isError ? (
            <p className="text-xs text-destructive">
              {attachTag.error.message}
            </p>
          ) : null}
        </PopoverContent>
      </Popover>
    </div>
  )
}
