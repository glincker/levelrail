import { useRef, useState, type DragEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  FileZipIcon,
  FolderOpenIcon,
  UploadSimpleIcon,
} from '@phosphor-icons/react/dist/ssr'
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
        'flex flex-col items-center gap-3 rounded-lg border border-dashed border-border px-4 py-8 text-center transition-colors',
        dragging && 'border-brand bg-muted',
      )}
    >
      <UploadSimpleIcon
        className="size-6 text-muted-foreground"
        aria-hidden="true"
      />
      <p className="text-sm font-medium text-foreground">
        {t('drop.dropHere')}
      </p>
      <p className="max-w-md text-xs text-muted-foreground">{t('drop.hint')}</p>
      <div className="flex flex-wrap justify-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled}
          onClick={() => folderRef.current?.click()}
        >
          <FolderOpenIcon aria-hidden="true" />
          {t('drop.chooseFolder')}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled}
          onClick={() => zipRef.current?.click()}
        >
          <FileZipIcon aria-hidden="true" />
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
  )
}
