import {
  useEffect,
  useMemo,
  useRef,
  type KeyboardEvent,
  type MouseEvent,
} from 'react'
import { useNavigate } from '@tanstack/react-router'
import { renderDocsMarkdown } from '../lib/renderDocsMarkdown'
import { renderMermaidDiagrams } from '../lib/renderMermaidDiagrams'
import type { DocsManifest } from '../types/docs'

// Renders one bundled doc's raw markdown as sanitized-by-construction HTML
// (marked's own escaping, plus every dynamic string this app injects
// itself, escaped in vitepressContainers.ts): first-party content shipped
// in this repo, never user input, so no separate HTML sanitizer is
// pulled in for this. Must not be reused for anything that renders
// untrusted markdown.
export function DocsRenderer({
  markdown,
  currentFile,
  manifest,
  docsBaseUrl,
}: {
  markdown: string
  currentFile: string
  manifest: DocsManifest
  docsBaseUrl: string
}) {
  const navigate = useNavigate()
  const containerRef = useRef<HTMLDivElement>(null)
  const html = useMemo(
    () => renderDocsMarkdown(markdown, currentFile, manifest, docsBaseUrl),
    [markdown, currentFile, manifest, docsBaseUrl],
  )

  // Mermaid needs a live DOM node to measure and render into, so it runs
  // here rather than in renderDocsMarkdown. Re-runs on theme toggle too
  // (light/dark are separate mermaid themes, not automatic) via a
  // MutationObserver on the <html> class, the same signal ThemeProvider
  // itself writes to.
  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const renderForCurrentTheme = () => {
      const isDark = document.documentElement.classList.contains('dark')
      void renderMermaidDiagrams(container, isDark)
    }

    renderForCurrentTheme()

    const observer = new MutationObserver(renderForCurrentTheme)
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    })
    return () => {
      observer.disconnect()
    }
  }, [html])

  // The rendered anchors are real <a> elements, so keyboard activation
  // (Enter) already reaches them natively; this delegates both a click
  // and a keyboard-triggered click on one of them to the router instead
  // of a full page navigation.
  function internalHrefFromTarget(target: EventTarget | null): string | null {
    if (!(target instanceof Element)) return null
    const anchor = target.closest('a[data-internal-doc="true"]')
    if (!(anchor instanceof HTMLAnchorElement)) return null
    return anchor.getAttribute('href')
  }

  function handleClick(event: MouseEvent<HTMLDivElement>) {
    const href = internalHrefFromTarget(event.target)
    if (!href) return
    event.preventDefault()
    void navigate({ to: href })
  }

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key !== 'Enter') return
    const href = internalHrefFromTarget(event.target)
    if (!href) return
    event.preventDefault()
    void navigate({ to: href })
  }

  return (
    <div
      ref={containerRef}
      className="prose-docs max-w-none text-sm leading-relaxed text-foreground [&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-xs [&_h2]:mt-8 [&_h2]:mb-3 [&_h2]:text-base [&_h3]:mt-6 [&_h3]:mb-2 [&_h3]:text-sm [&_li]:my-1 [&_ol]:my-3 [&_ol]:list-decimal [&_ol]:pl-5 [&_p]:my-3 [&_pre]:my-3 [&_pre]:overflow-x-auto [&_pre]:rounded-lg [&_pre]:bg-muted [&_pre]:p-3 [&_pre_code]:bg-transparent [&_pre_code]:p-0 [&_table]:my-3 [&_table]:w-full [&_td]:border [&_td]:border-border [&_td]:p-2 [&_th]:border [&_th]:border-border [&_th]:p-2 [&_th]:text-left [&_ul]:my-3 [&_ul]:list-disc [&_ul]:pl-5"
      onClick={handleClick}
      onKeyDown={handleKeyDown}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  )
}
