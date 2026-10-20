import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { DARK_THEME, LIGHT_THEME, isDark } from './cliConsoleTheme'
import { findNameRanges } from './cliConsoleQuickActions'

interface MountOptions {
  onData: (data: string) => void
  appNames: () => string[]
  onSelectApp: (name: string) => void
}

/** Opens an xterm in container, fitted and themed; the result tears it all down. */
export function mountConsoleTerminal(
  container: HTMLElement,
  { onData, appNames, onSelectApp }: MountOptions,
): { term: Terminal; dispose: () => void } {
  const term = new Terminal({
    cursorBlink: true,
    fontFamily:
      'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    fontSize: 13,
    theme: isDark() ? DARK_THEME : LIGHT_THEME,
  })
  const fit = new FitAddon()
  term.loadAddon(fit)
  term.open(container)
  fit.fit()
  // Links decorate output only; activating one just selects the app.
  const linkSub = term.registerLinkProvider({
    provideLinks(y, callback) {
      const names = appNames()
      const text = term.buffer.active.getLine(y - 1)?.translateToString(true)
      if (!text || names.length === 0) return callback(undefined)
      callback(
        findNameRanges(text, names).map((r) => ({
          text: r.name,
          range: { start: { x: r.start + 1, y }, end: { x: r.end, y } },
          activate: () => {
            onSelectApp(r.name)
          },
        })),
      )
    },
  })
  const dataSub = term.onData(onData)
  let frame = 0
  const refit = () => {
    cancelAnimationFrame(frame)
    frame = requestAnimationFrame(() => {
      if (container.clientWidth === 0 || container.clientHeight === 0) return
      try {
        fit.fit()
      } catch {
        // mid-transition; the next observed size change re-fits
      }
    })
  }
  const observer = new ResizeObserver(refit)
  observer.observe(container)
  if (container.parentElement) observer.observe(container.parentElement)
  window.addEventListener('resize', refit)
  refit()
  const theme = new MutationObserver(() => {
    term.options.theme = isDark() ? DARK_THEME : LIGHT_THEME
  })
  theme.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class'],
  })
  return {
    term,
    dispose: () => {
      cancelAnimationFrame(frame)
      window.removeEventListener('resize', refit)
      observer.disconnect()
      theme.disconnect()
      dataSub.dispose()
      linkSub.dispose()
      term.dispose()
    },
  }
}
