import { TagIcon } from '@phosphor-icons/react/dist/ssr'
import { useTags } from '../queries/tags'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'

// Filter-by-tag control for the apps list route: client-side only
// (AppListPage.tsx's own doc comment explains why: GET /api/v1/apps
// already returns every app in one response with no server-side
// pagination to extend with a tag query param, the exact "already
// small/paginated appropriately" case this feature's own scoping calls
// out). Selection is OR semantics (an app matching any checked tag
// passes), the ordinary "show me anything in this bucket or that one"
// reading of a multi-select filter.
export function TagFilter({
  selected,
  onChange,
}: {
  selected: string[]
  onChange: (next: string[]) => void
}) {
  const { data: tags } = useTags()

  const toggle = (name: string) => {
    onChange(
      selected.includes(name)
        ? selected.filter((t) => t !== name)
        : [...selected, name],
    )
  }

  if (!tags || tags.length === 0) {
    return null
  }

  return (
    <Popover>
      <PopoverTrigger
        render={<Button type="button" size="sm" variant="outline" />}
      >
        <TagIcon />
        Tags
        {selected.length > 0 ? (
          <Badge variant="muted" className="ml-1 px-1.5 text-[11px]">
            {selected.length}
          </Badge>
        ) : null}
      </PopoverTrigger>
      <PopoverContent align="end" className="w-56">
        <div className="flex flex-col gap-1.5">
          {tags.map((tag) => (
            <Label
              key={tag.id}
              className="cursor-pointer rounded px-1 py-0.5 font-normal hover:bg-muted/60"
            >
              <Checkbox
                checked={selected.includes(tag.name)}
                onCheckedChange={() => {
                  toggle(tag.name)
                }}
              />
              {tag.name}
            </Label>
          ))}
        </div>
        {selected.length > 0 ? (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="mt-2 w-full"
            onClick={() => {
              onChange([])
            }}
          >
            Clear filter
          </Button>
        ) : null}
      </PopoverContent>
    </Popover>
  )
}
