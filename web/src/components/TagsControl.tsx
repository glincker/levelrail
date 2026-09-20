import { useState } from 'react'
import { TagIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { useAttachAppTag, useDetachAppTag, useTags } from '../queries/tags'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

// A tag chip's own "x" button needs the tag's ID to call DELETE
// /api/v1/apps/{name}/tags/{id}, but AppDetail.tags (what this
// component is handed) is only ever a list of names (see that field's
// own doc comment: attach/detach round-trip through their own dedicated
// endpoints, never the general PUT). Tag names are globally unique
// (migrations/0106_tags.sql), so the global tag list (useTags) is
// enough to resolve a name back to the ID a detach call needs, without
// a second per-app endpoint that returns IDs alongside names.
export function TagsControl({
  appName,
  tags,
}: {
  appName: string
  tags?: string[]
}) {
  const [value, setValue] = useState('')
  const { data: allTags } = useTags()
  const attachTag = useAttachAppTag(appName)
  const detachTag = useDetachAppTag(appName)

  const idByName = new Map((allTags ?? []).map((t) => [t.name, t.id]))
  const appliedTags = tags ?? []

  const onAdd = () => {
    const name = value.trim()
    if (!name) {
      return
    }
    attachTag.mutate(name, { onSuccess: () => setValue('') })
  }

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-1.5">
        {appliedTags.length === 0 ? (
          <span className="text-sm text-muted-foreground">No tags.</span>
        ) : (
          appliedTags.map((name) => {
            const id = idByName.get(name)
            return (
              <Badge key={name} variant="outline" className="gap-1 pr-1">
                <TagIcon className="size-3" />
                {name}
                <button
                  type="button"
                  className="ml-0.5 rounded-sm text-muted-foreground hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
                  disabled={!id || detachTag.isPending}
                  onClick={() => {
                    if (id) {
                      detachTag.mutate(id)
                    }
                  }}
                >
                  <XIcon className="size-3" />
                  <span className="sr-only">Remove tag {name}</span>
                </button>
              </Badge>
            )
          })
        )}
      </div>
      <div className="flex items-center gap-2">
        <Input
          value={value}
          onChange={(e) => {
            setValue(e.target.value)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              onAdd()
            }
          }}
          placeholder="Add a tag"
          list="app-tags-suggestions"
          aria-label="New tag name"
          className="h-8 max-w-48 text-sm"
        />
        <datalist id="app-tags-suggestions">
          {(allTags ?? []).map((t) => (
            <option key={t.id} value={t.name} />
          ))}
        </datalist>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={!value.trim() || attachTag.isPending}
          onClick={onAdd}
        >
          {attachTag.isPending ? 'Adding...' : 'Add'}
        </Button>
      </div>
      {attachTag.isError ? (
        <p className="text-xs text-destructive">{attachTag.error.message}</p>
      ) : null}
      {detachTag.isError ? (
        <p className="text-xs text-destructive">{detachTag.error.message}</p>
      ) : null}
    </div>
  )
}
