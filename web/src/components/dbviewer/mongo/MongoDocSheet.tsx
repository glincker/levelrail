import { useTranslation } from 'react-i18next'
import {
  ChevronLeftGlassIcon,
  ChevronRightGlassIcon,
  CopyGlassIcon,
} from '@thesvg/ui-react'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Button } from '@/components/ui/button'
import { prettyDocument } from '../../../lib/mongoExplorer'
import type { JsonValue } from '../../../types/databaseMongo'
import { useCopy } from '../explorer/useCopy'
import { JsonTree } from './JsonTree'

const OPEN_ALL = { version: 1, open: true }

export function MongoDocSheet({
  docs,
  index,
  onIndexChange,
  title,
}: {
  docs: JsonValue[]
  index: number | null
  onIndexChange: (i: number | null) => void
  title: string
}) {
  const { t } = useTranslation('databases')
  const copy = useCopy()
  const doc = index === null ? undefined : docs[index]
  return (
    <Sheet
      open={doc !== undefined}
      onOpenChange={(o) => {
        if (!o) onIndexChange(null)
      }}
    >
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>
            {t('viewer.grid.rowOf', {
              row: (index ?? 0) + 1,
              total: docs.length,
            })}
          </SheetDescription>
          <div className="flex gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={(index ?? 0) <= 0}
              onClick={() => {
                onIndexChange((index ?? 0) - 1)
              }}
            >
              <ChevronLeftGlassIcon className="size-5" aria-hidden="true" />
              {t('viewer.grid.previousRow')}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={(index ?? 0) >= docs.length - 1}
              onClick={() => {
                onIndexChange((index ?? 0) + 1)
              }}
            >
              {t('viewer.grid.nextRow')}
              <ChevronRightGlassIcon className="size-5" aria-hidden="true" />
            </Button>
            {doc !== undefined ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => {
                  copy(prettyDocument(doc), 'viewer.grid.copied_json')
                }}
              >
                <CopyGlassIcon className="size-5" aria-hidden="true" />
                {t('viewer.mongo.docs.copyDoc')}
              </Button>
            ) : null}
          </div>
        </SheetHeader>
        <div className="px-4 pb-6">
          {doc !== undefined ? (
            <JsonTree value={doc} signal={OPEN_ALL} />
          ) : null}
        </div>
      </SheetContent>
    </Sheet>
  )
}
