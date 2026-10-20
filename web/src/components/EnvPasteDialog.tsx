import { useMemo, useRef, useState, type DragEvent } from 'react'
import {
  ClipboardTextIcon,
  UploadSimpleIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  classifyEnvImport,
  parseEnvBlock,
  type EnvEntry,
  type EnvImportRow,
} from '../lib/envParse'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

type ConflictChoice = 'overwrite' | 'keep'

const STATUS_LABEL: Record<EnvImportRow['status'], string> = {
  new: 'New',
  changed: 'Changed',
  unchanged: 'Unchanged',
}

function ImportPreviewTable({
  rows,
  conflict,
}: {
  rows: EnvImportRow[]
  conflict: ConflictChoice
}) {
  return (
    <div className="max-h-56 overflow-auto rounded-md border border-border">
      <table className="w-full text-left text-xs" aria-label="Import preview">
        <thead className="sticky top-0 bg-muted text-muted-foreground">
          <tr>
            <th className="px-2 py-1 font-medium">Key</th>
            <th className="px-2 py-1 font-medium">Value</th>
            <th className="px-2 py-1 font-medium">Result</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.key} className="border-t border-border">
              <td className="px-2 py-1 font-mono">{row.key}</td>
              <td className="max-w-[12rem] truncate px-2 py-1 font-mono text-muted-foreground">
                {row.status === 'changed' && row.previous !== undefined ? (
                  <>
                    <span className="line-through">{row.previous}</span>
                    {' → '}
                  </>
                ) : null}
                {row.value}
              </td>
              <td className="px-2 py-1">
                <Badge
                  variant={row.status === 'unchanged' ? 'muted' : 'outline'}
                >
                  {row.status === 'changed' && conflict === 'keep'
                    ? 'Kept existing'
                    : STATUS_LABEL[row.status]}
                </Badge>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// Paste-or-upload .env dialog. Parsed rows are previewed against the form's
// current values (new, changed, unchanged) and only staged into the form
// once the user clicks Import, so nothing is saved by this dialog itself.
export function EnvPasteDialog({
  placeholder,
  getCurrent,
  onApply,
}: {
  placeholder: string
  getCurrent: () => Record<string, string>
  onApply: (entries: EnvEntry[]) => void
}) {
  const [open, setOpen] = useState(false)
  const [text, setText] = useState('')
  const [conflict, setConflict] = useState<ConflictChoice>('overwrite')
  const [isDragging, setIsDragging] = useState(false)
  const [fileError, setFileError] = useState<string | undefined>()
  const fileInputRef = useRef<HTMLInputElement>(null)

  const rows = useMemo(
    () => (open ? classifyEnvImport(getCurrent(), parseEnvBlock(text)) : []),
    // getCurrent reads live form state; re-evaluate when the text or dialog changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [open, text],
  )
  const applicable = rows.filter(
    (r) =>
      r.status === 'new' ||
      (r.status === 'changed' && conflict === 'overwrite'),
  )
  const counts = {
    new: rows.filter((r) => r.status === 'new').length,
    changed: rows.filter((r) => r.status === 'changed').length,
    unchanged: rows.filter((r) => r.status === 'unchanged').length,
  }

  const reset = () => {
    setText('')
    setFileError(undefined)
    setIsDragging(false)
    setConflict('overwrite')
  }

  const importEnvFile = (file: File) => {
    setFileError(undefined)
    const reader = new FileReader()
    reader.onload = () => {
      const content = typeof reader.result === 'string' ? reader.result : ''
      if (parseEnvBlock(content).length === 0) {
        setFileError(`No key=value pairs found in ${file.name}.`)
        return
      }
      setText(content)
    }
    reader.onerror = () => {
      setFileError(`Could not read ${file.name}.`)
    }
    reader.readAsText(file)
  }

  const handleDrop = (e: DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    setIsDragging(false)
    const file = e.dataTransfer.files?.[0]
    if (file) importEnvFile(file)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) reset()
      }}
    >
      <DialogTrigger
        render={<Button type="button" variant="outline" size="sm" />}
      >
        <ClipboardTextIcon />
        Paste .env
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Paste .env</DialogTitle>
          <DialogDescription>
            Paste raw .env-format text or load a .env file. Comments, an export
            prefix and quotes are handled, and multiline values are supported.
            Review the preview, then Import to stage the rows. Click Save
            variables afterward to apply them.
          </DialogDescription>
        </DialogHeader>
        <div
          data-testid="env-file-dropzone"
          className={cn(
            'flex flex-col items-center gap-2 rounded-md border border-dashed p-4 text-center transition-colors',
            isDragging ? 'border-primary bg-accent' : 'border-border',
          )}
          onDragOver={(e) => {
            e.preventDefault()
            setIsDragging(true)
          }}
          onDragLeave={() => {
            setIsDragging(false)
          }}
          onDrop={handleDrop}
        >
          <UploadSimpleIcon className="size-5 text-muted-foreground" />
          <p className="text-sm text-muted-foreground">
            Drag and drop a .env file here, or
          </p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => {
              fileInputRef.current?.click()
            }}
          >
            Browse file
          </Button>
          <input
            ref={fileInputRef}
            type="file"
            accept=".env,text/plain"
            className="sr-only"
            aria-label="Upload .env file"
            onChange={(e) => {
              const file = e.target.files?.[0]
              if (file) importEnvFile(file)
              e.target.value = ''
            }}
          />
        </div>
        {fileError ? (
          <Alert variant="destructive">
            <AlertDescription>{fileError}</AlertDescription>
          </Alert>
        ) : null}
        <Textarea
          value={text}
          onChange={(e) => {
            setText(e.target.value)
          }}
          className="min-h-32 font-mono"
          placeholder={placeholder}
          aria-label="Paste .env content"
          autoFocus
        />
        {rows.length > 0 ? (
          <div className="space-y-2">
            <p className="text-xs text-muted-foreground">
              {counts.new} new, {counts.changed} changed, {counts.unchanged}{' '}
              unchanged
            </p>
            <ImportPreviewTable rows={rows} conflict={conflict} />
            {counts.changed > 0 ? (
              <fieldset className="flex flex-wrap items-center gap-4 text-sm">
                <legend className="sr-only">Conflict handling</legend>
                {(
                  [
                    ['overwrite', 'Overwrite changed values'],
                    ['keep', 'Keep existing values'],
                  ] as const
                ).map(([value, label]) => (
                  <label key={value} className="flex items-center gap-1.5">
                    <input
                      type="radio"
                      name="env-import-conflict"
                      value={value}
                      checked={conflict === value}
                      onChange={() => {
                        setConflict(value)
                      }}
                    />
                    {label}
                  </label>
                ))}
              </fieldset>
            ) : null}
          </div>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setOpen(false)
              reset()
            }}
          >
            Cancel
          </Button>
          <Button
            type="button"
            disabled={applicable.length === 0}
            onClick={() => {
              onApply(applicable.map(({ key, value }) => ({ key, value })))
              setOpen(false)
              reset()
            }}
          >
            Import
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
