import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CodeGlassIcon,
  DownloadGlassIcon,
  ListGlassIcon,
  PlusGlassIcon,
  ShieldGlassIcon,
  UploadGlassIcon,
} from '@thesvg/ui-react'
import type { ExportFormat } from '../../queries/envProtection'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Switch } from '@/components/ui/switch'

export interface EnvEditorToolbarProps {
  mode: 'table' | 'bulk'
  dirty: boolean
  canFullExport: boolean
  writeOnly: boolean
  writeOnlyPending: boolean
  onAdd: () => void
  onToggleMode: () => void
  onImport: (text: string) => void
  onExport: (format: ExportFormat, full: boolean) => void
  onWriteOnly: (value: boolean) => void
  onReview: () => void
  onDiscard: () => void
}

export function EnvEditorToolbar(props: EnvEditorToolbarProps) {
  const { t } = useTranslation('envEditor')
  const fileRef = useRef<HTMLInputElement>(null)

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        type="button"
        size="sm"
        onClick={props.onAdd}
        disabled={props.mode === 'bulk'}
      >
        <PlusGlassIcon size="1.2em" />
        {t('toolbar.add')}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={props.onToggleMode}
      >
        {props.mode === 'table' ? (
          <CodeGlassIcon size="1.2em" />
        ) : (
          <ListGlassIcon size="1.2em" />
        )}
        {props.mode === 'table' ? t('toolbar.bulk') : t('toolbar.table')}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={() => {
          fileRef.current?.click()
        }}
      >
        <UploadGlassIcon size="1.2em" />
        {t('toolbar.import')}
      </Button>
      <input
        ref={fileRef}
        type="file"
        accept=".env,text/plain"
        className="hidden"
        aria-hidden
        tabIndex={-1}
        onChange={(e) => {
          const file = e.target.files?.[0]
          e.target.value = ''
          if (file) {
            void file.text().then(props.onImport)
          }
        }}
      />
      <DropdownMenu>
        <DropdownMenuTrigger
          render={<Button type="button" size="sm" variant="outline" />}
        >
          <DownloadGlassIcon size="1.2em" />
          {t('toolbar.export')}
        </DropdownMenuTrigger>
        <DropdownMenuContent>
          <DropdownMenuItem
            onClick={() => {
              props.onExport('dotenv', false)
            }}
          >
            {t('toolbar.exportMasked')}
          </DropdownMenuItem>
          {props.canFullExport ? (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                onClick={() => {
                  props.onExport('dotenv', true)
                }}
              >
                {t('toolbar.exportFull')}
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => {
                  props.onExport('json', true)
                }}
              >
                {t('toolbar.exportFullJson')}
              </DropdownMenuItem>
            </>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
      <Popover>
        <PopoverTrigger
          render={<Button type="button" size="sm" variant="outline" />}
        >
          <ShieldGlassIcon size="1.2em" />
          {t('toolbar.protection')}
        </PopoverTrigger>
        <PopoverContent className="w-80">
          <PopoverHeader>
            <PopoverTitle>{t('toolbar.protection')}</PopoverTitle>
            <PopoverDescription>
              {t('policy.writeOnlyToggleHint')}
            </PopoverDescription>
          </PopoverHeader>
          <label className="flex items-center justify-between gap-3 text-sm">
            {t('policy.writeOnlyToggle')}
            <Switch
              checked={props.writeOnly}
              disabled={props.writeOnlyPending}
              onCheckedChange={(value) => {
                props.onWriteOnly(value)
              }}
            />
          </label>
        </PopoverContent>
      </Popover>
      {props.dirty ? (
        <div className="ml-auto flex gap-2">
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={props.onDiscard}
          >
            {t('toolbar.discard')}
          </Button>
          <Button type="button" size="sm" onClick={props.onReview}>
            {t('toolbar.review')}
          </Button>
        </div>
      ) : null}
    </div>
  )
}
