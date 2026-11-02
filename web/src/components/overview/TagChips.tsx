import { useTranslation } from 'react-i18next'
import { XGlassIcon } from '@thesvg/ui-react'
import { useDetachAppTag, useTags } from '../../queries/tags'
import { Badge } from '@/components/ui/badge'

/** TagChips lists the app's tags; adding one lives in the overflow menu. */
export function TagChips({
  appName,
  tags,
}: {
  appName: string
  tags?: string[]
}) {
  const { t } = useTranslation('common')
  const { data: allTags } = useTags()
  const detachTag = useDetachAppTag(appName)
  const idByName = new Map((allTags ?? []).map((tag) => [tag.name, tag.id]))
  if (!tags || tags.length === 0) return null

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {tags.map((name) => {
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
              <XGlassIcon className="size-4.5" aria-hidden="true" />
              <span className="sr-only">
                {t('overviewHero.removeTag', { name })}
              </span>
            </button>
          </Badge>
        )
      })}
    </div>
  )
}
