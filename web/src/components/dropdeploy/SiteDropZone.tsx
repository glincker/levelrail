import { useRef, useState, type DragEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  FileArchiveGlassIcon,
  FolderGlassIcon,
  UploadGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import {
  selectionFromDataTransfer,
  selectionFromFileList,
  type DropSelection,
} from '../../lib/zipSite'

const DIRECTORY_INPUT_ATTRS = { webkitdirectory: '', directory: '' }

/** Drag target plus folder and zip pickers. Reports a selection, or null when nothing publishable was chosen. */
export function SiteDropZone({
  onSelect,
  disabled,
}: {
  onSelect: (selection: DropSelection | null) => void
  disabled?: boolean
}) {
  const { t } = useTranslation('newApp')
  const [dragging, setDragging] = useState(false)
  const folderRef = useRef<HTMLInputElement>(null)
  const zipRef = useRef<HTMLInputElement>(null)

  const onDrop = async (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    setDragging(false)
    if (disabled) return
    onSelect(await selectionFromDataTransfer(e.dataTransfer))
  }

  return (
    <div className="@container">
      <div
        role="group"
        aria-label={t('drop.zoneLabel')}
        data-testid="site-drop-zone"
        onDragOver={(e) => {
          e.preventDefault()
          if (!disabled) setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => void onDrop(e)}
        className={cn(
          'flex min-h-36 flex-col items-center justify-center gap-3 rounded-lg border-2 border-dashed border-border px-4 py-5 text-center transition-colors @lg:min-h-0 @lg:flex-row @lg:justify-start @lg:py-4 @lg:text-left',
          dragging && 'border-brand bg-muted ring-2 ring-brand/30',
        )}
      >
        <UploadGlassIcon
          className="size-8 text-muted-foreground"
          aria-hidden="true"
        />
        <div className="min-w-0 @lg:flex-1">
          <p className="text-sm font-medium text-foreground @lg:text-left">
            {t('drop.dropHere')}
          </p>
          <p className="line-clamp-2 max-w-md text-xs text-muted-foreground">
            {t('drop.hint')}
          </p>
        </div>
        <div className="flex w-full flex-col justify-center gap-2 @xs:w-auto @xs:flex-row">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={disabled}
            onClick={() => folderRef.current?.click()}
          >
            <FolderGlassIcon size="1.4em" aria-hidden="true" />
            {t('drop.chooseFolder')}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={disabled}
            onClick={() => zipRef.current?.click()}
          >
            <FileArchiveGlassIcon size="1.4em" aria-hidden="true" />
            {t('drop.chooseZip')}
          </Button>
        </div>
        <input
          ref={folderRef}
          type="file"
          multiple
          hidden
          data-testid="site-folder-input"
          aria-label={t('drop.chooseFolder')}
          {...DIRECTORY_INPUT_ATTRS}
          onChange={(e) => {
            onSelect(
              e.target.files ? selectionFromFileList(e.target.files) : null,
            )
            e.target.value = ''
          }}
        />
        <input
          ref={zipRef}
          type="file"
          accept=".zip,application/zip"
          hidden
          data-testid="site-zip-input"
          aria-label={t('drop.chooseZip')}
          onChange={(e) => {
            onSelect(
              e.target.files ? selectionFromFileList(e.target.files) : null,
            )
            e.target.value = ''
          }}
        />
      </div>
    </div>
  )
}
