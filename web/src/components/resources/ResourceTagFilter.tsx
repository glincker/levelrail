import { useTranslation } from 'react-i18next'
import { TagGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { useTags } from '@/queries/tags'

// AND semantics: the explorer API returns only resources carrying every selected tag.
export function ResourceTagFilter({
  selected,
  onChange,
}: {
  selected: string[]
  onChange: (next: string[]) => void
}) {
  const { t } = useTranslation('resources')
  const { data: tags } = useTags()
  const toggle = (name: string) =>
    onChange(
      selected.includes(name)
        ? selected.filter((n) => n !== name)
        : [...selected, name],
    )
  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            type="button"
            size="sm"
            variant="outline"
            aria-label={t('tags.filterLabel')}
          />
        }
      >
        <TagGlassIcon size="1.4em" />
        {t('tags.filter')}
        {selected.length > 0 ? (
          <Badge variant="muted" className="ml-1 px-1.5 text-[11px]">
            {selected.length}
          </Badge>
        ) : null}
      </PopoverTrigger>
      <PopoverContent align="end" className="w-60">
        {!tags || tags.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('tags.none')}</p>
        ) : (
          <div className="flex max-h-64 flex-col gap-1.5 overflow-auto">
            {tags.map((tag) => (
              <Label
                key={tag.id}
                className="cursor-pointer rounded px-1 py-0.5 font-normal hover:bg-muted/60"
              >
                <Checkbox
                  checked={selected.includes(tag.name)}
                  onCheckedChange={() => toggle(tag.name)}
                />
                {tag.name}
              </Label>
            ))}
          </div>
        )}
        {selected.length > 0 ? (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="mt-2 w-full"
            onClick={() => onChange([])}
          >
            {t('tags.clear')}
          </Button>
        ) : null}
      </PopoverContent>
    </Popover>
  )
}
