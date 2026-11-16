import { ChevronDownGlassIcon, ChevronRightGlassIcon } from '@thesvg/ui-react'
import { useEffect, useState, type ReactNode } from 'react'
import {
  RUN_SECTION_OPEN_EVENT,
  readSectionOpen,
  sectionDomId,
  sectionFromHash,
  writeSectionOpen,
  type RunSectionId,
} from '../../lib/runSections'

const FLASH_MS = 1200

/** One collapsible block of the run page; open state is remembered. */
export function RunSection({
  id,
  title,
  chip,
  defaultOpen = true,
  children,
}: {
  id: RunSectionId
  title: string
  chip?: ReactNode
  defaultOpen?: boolean
  children: ReactNode
}) {
  const [open, setOpen] = useState(() => {
    const fromHash =
      typeof window !== 'undefined' && sectionFromHash(window.location.hash)
    return fromHash === id ? true : readSectionOpen(id, defaultOpen)
  })
  const [flash, setFlash] = useState(false)

  useEffect(() => {
    let timer = 0
    const onOpen = (e: Event) => {
      if (!(e instanceof CustomEvent) || e.detail !== id) return
      setOpen(true)
      setFlash(true)
      window.clearTimeout(timer)
      timer = window.setTimeout(() => {
        setFlash(false)
      }, FLASH_MS)
    }
    window.addEventListener(RUN_SECTION_OPEN_EVENT, onOpen)
    return () => {
      window.removeEventListener(RUN_SECTION_OPEN_EVENT, onOpen)
      window.clearTimeout(timer)
    }
  }, [id])

  const toggle = () => {
    setOpen((prev) => {
      writeSectionOpen(id, !prev)
      return !prev
    })
  }
  const Caret = open ? ChevronDownGlassIcon : ChevronRightGlassIcon
  const bodyId = `${sectionDomId(id)}-body`

  return (
    <section
      id={sectionDomId(id)}
      aria-label={title}
      className={`scroll-mt-16 rounded-lg border border-border bg-card ${flash ? 'ring-2 ring-ring' : ''}`}
    >
      <h2 className="m-0">
        <button
          type="button"
          aria-expanded={open}
          aria-controls={bodyId}
          onClick={toggle}
          className="flex w-full items-center gap-2 rounded-lg px-4 py-3 text-left focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        >
          <Caret
            className="size-4 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
          <span className="text-sm font-semibold text-foreground">{title}</span>
          {chip ? (
            <span className="ml-auto text-xs text-muted-foreground">
              {chip}
            </span>
          ) : null}
        </button>
      </h2>
      {open ? (
        <div id={bodyId} className="space-y-4 px-4 pb-4">
          {children}
        </div>
      ) : null}
    </section>
  )
}
