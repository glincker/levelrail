import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CaretDownIcon,
  DownloadSimpleIcon,
  PencilSimpleIcon,
  PlusIcon,
  SquaresFourIcon,
  TextTIcon,
  TrashIcon,
  UploadSimpleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

export interface ManageMenuProps {
  /** The selected saved board, or null on Start and Metrics. */
  board: { editable: boolean } | null
  editing: boolean
  busy: boolean
  onNew: () => void
  onStarter: () => void
  onImportFile: (file: File) => void
  onEdit: () => void
  onRename: () => void
  onExport: () => void
  onDelete: () => void
}

/** Board management that used to live on the Dashboards page: create, import, and edit the selected board. */
export function ManageMenu({
  board,
  editing,
  busy,
  onNew,
  onStarter,
  onImportFile,
  onEdit,
  onRename,
  onExport,
  onDelete,
}: ManageMenuProps) {
  const { t } = useTranslation('dashboard')
  const fileRef = useRef<HTMLInputElement>(null)
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button type="button" variant="ghost" size="sm" disabled={busy} />
          }
        >
          {t('manage.label')}
          <CaretDownIcon aria-hidden="true" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onClick={onNew}>
            <PlusIcon aria-hidden="true" />
            {t('manage.new')}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={onStarter}>
            <SquaresFourIcon aria-hidden="true" />
            {t('manage.starter')}
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={() => {
              fileRef.current?.click()
            }}
          >
            <UploadSimpleIcon aria-hidden="true" />
            {t('manage.import')}
          </DropdownMenuItem>
          {board ? (
            <>
              <DropdownMenuSeparator />
              {board.editable ? (
                <DropdownMenuItem onClick={onEdit}>
                  <PencilSimpleIcon aria-hidden="true" />
                  {editing ? t('manage.doneEditing') : t('manage.edit')}
                </DropdownMenuItem>
              ) : null}
              {board.editable ? (
                <DropdownMenuItem onClick={onRename}>
                  <TextTIcon aria-hidden="true" />
                  {t('manage.rename')}
                </DropdownMenuItem>
              ) : null}
              <DropdownMenuItem onClick={onExport}>
                <DownloadSimpleIcon aria-hidden="true" />
                {t('manage.export')}
              </DropdownMenuItem>
              {board.editable ? (
                <DropdownMenuItem onClick={onDelete}>
                  <TrashIcon aria-hidden="true" />
                  {t('manage.delete')}
                </DropdownMenuItem>
              ) : null}
            </>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
      <input
        ref={fileRef}
        type="file"
        accept="application/json,.json"
        className="sr-only"
        tabIndex={-1}
        aria-label={t('manage.importFile')}
        onChange={(e) => {
          const f = e.target.files?.[0]
          if (f) onImportFile(f)
          e.target.value = ''
        }}
      />
    </>
  )
}
