import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { UploadGlassIcon } from '@thesvg/ui-react'
import { NewSiteDialog } from '../pages/NewSiteDialog'
import { usePagesSites } from '../../queries/pagesSites'
import {
  selectionFromDataTransfer,
  type DropSelection,
} from '../../lib/zipSite'

function hasFiles(e: DragEvent): boolean {
  return Array.from(e.dataTransfer?.types ?? []).includes('Files')
}

/** Drop a built site folder or zip anywhere on the dashboard to open the publish dialog with it. */
export function DashboardSiteDrop() {
  const { t } = useTranslation('pagesSites')
  const sites = usePagesSites()
  const [dragging, setDragging] = useState(false)
  const [selection, setSelection] = useState<DropSelection | null>(null)
  const [open, setOpen] = useState(false)
  const openRef = useRef(false)

  useEffect(() => {
    openRef.current = open
  }, [open])

  useEffect(() => {
    // Depth counter: dragenter/leave fire for every child element crossed.
    let depth = 0
    const enter = (e: DragEvent) => {
      if (openRef.current || !hasFiles(e)) return
      depth += 1
      setDragging(true)
    }
    const over = (e: DragEvent) => {
      if (openRef.current || !hasFiles(e)) return
      e.preventDefault()
    }
    const leave = (e: DragEvent) => {
      if (!hasFiles(e)) return
      depth = Math.max(0, depth - 1)
      if (depth === 0) setDragging(false)
    }
    const drop = (e: DragEvent) => {
      if (openRef.current || !hasFiles(e) || !e.dataTransfer) return
      e.preventDefault()
      depth = 0
      setDragging(false)
      void selectionFromDataTransfer(e.dataTransfer).then((picked) => {
        if (!picked) return
        setSelection(picked)
        setOpen(true)
      })
    }
    window.addEventListener('dragenter', enter)
    window.addEventListener('dragover', over)
    window.addEventListener('dragleave', leave)
    window.addEventListener('drop', drop)
    return () => {
      window.removeEventListener('dragenter', enter)
      window.removeEventListener('dragover', over)
      window.removeEventListener('dragleave', leave)
      window.removeEventListener('drop', drop)
    }
  }, [])

  return (
    <>
      {dragging ? (
        <div
          data-testid="dashboard-site-drop"
          className="pointer-events-none fixed inset-0 z-50 flex items-center justify-center bg-background/80"
        >
          <div className="flex items-center gap-3 rounded-lg border-2 border-dashed border-brand bg-card px-6 py-5 text-sm font-medium">
            <UploadGlassIcon
              className="size-6 text-muted-foreground"
              aria-hidden="true"
            />
            {t('dashDrop.prompt')}
          </div>
        </div>
      ) : null}
      <NewSiteDialog
        key={selection?.label ?? 'none'}
        initialSelection={selection}
        open={open}
        onOpenChange={setOpen}
        existingNames={sites.data?.map((s) => s.name) ?? []}
      />
    </>
  )
}
