import { useEffect, useState } from 'react'
import { useDeployDetailT } from '../deployDetail/useDeployDetailT'
import {
  RUN_SECTIONS,
  goToSection,
  sectionDomId,
  sectionFromHash,
  type RunSectionId,
} from '../../lib/runSections'

/** Sticky anchor nav; a chip row that scrolls sideways on narrow screens. */
export function RunSectionNav() {
  const { t } = useDeployDetailT()
  const [active, setActive] = useState<RunSectionId>('summary')

  useEffect(() => {
    const els = RUN_SECTIONS.map((id) =>
      document.getElementById(sectionDomId(id)),
    ).filter((el): el is HTMLElement => el !== null)
    if (els.length === 0 || typeof IntersectionObserver === 'undefined') return
    const obs = new IntersectionObserver(
      (entries) => {
        const hit = entries.find((e) => e.isIntersecting)
        const id = hit?.target.id.replace('run-section-', '')
        if (id && RUN_SECTIONS.some((s) => s === id)) {
          setActive(id as RunSectionId)
        }
      },
      { rootMargin: '-15% 0px -70% 0px' },
    )
    els.forEach((el) => {
      obs.observe(el)
    })
    return () => {
      obs.disconnect()
    }
  }, [])

  // Deep link: open and scroll to the section named in the hash once.
  useEffect(() => {
    const target = sectionFromHash(window.location.hash)
    if (target) goToSection(target)
  }, [])

  return (
    <nav
      aria-label={t('sections.navLabel')}
      className="sticky top-0 z-10 -mx-1 flex gap-1 overflow-x-auto bg-background/90 px-1 py-1.5 backdrop-blur"
    >
      {RUN_SECTIONS.map((id) => (
        <button
          key={id}
          type="button"
          aria-current={active === id ? 'location' : undefined}
          onClick={() => {
            goToSection(id)
          }}
          className={`shrink-0 rounded-full px-3 py-1 text-xs font-medium focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none ${active === id ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:text-foreground'}`}
        >
          {t(`sections.${id}`)}
        </button>
      ))}
    </nav>
  )
}
