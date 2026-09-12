import { useCallback, useEffect, useRef, useState } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import {
  PlugsConnectedIcon,
  PlugsIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  describeTerminalExit,
  parseTerminalEvent,
  terminalSocketUrl,
} from '@/lib/terminalSocket'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

type TerminalState = 'idle' | 'connecting' | 'open' | 'closed'

// Two hand-picked palettes rather than reading CSS variables at runtime:
// xterm needs concrete hex colors for its 16 ANSI slots, which the
// design tokens do not define, and a terminal's colors are the program's
// output convention (red means error), not this dashboard's brand.
const DARK_THEME = {
  background: '#09090b',
  foreground: '#e4e4e7',
  cursor: '#e4e4e7',
  selectionBackground: '#3f3f46',
}

const LIGHT_THEME = {
  background: '#fafafa',
  foreground: '#18181b',
  cursor: '#18181b',
  selectionBackground: '#d4d4d8',
}

const STATE_LABEL: Record<TerminalState, string> = {
  idle: 'Not connected',
  connecting: 'Connecting',
  open: 'Connected',
  closed: 'Disconnected',
}

function prefersDarkTerminal(): boolean {
  return document.documentElement.classList.contains('dark')
}

/**
 * AppTerminal is a real interactive shell on the app's running
 * container: a PTY, keystroke-by-keystroke input, working arrow keys and
 * Ctrl-C, and a remote terminal that resizes with this one. It talks to
 * GET /api/v1/apps/{name}/terminal (internal/api/terminal.go) over a
 * WebSocket, which lib/terminalSocket.ts explains the choice of.
 *
 * A session opens on an explicit click, never on navigation: a shell can
 * read the plaintext secrets injected into the container, so opening one
 * should be something an operator did on purpose.
 */
export function AppTerminal({ name }: { name: string }) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const terminalRef = useRef<Terminal | null>(null)
  const [sessionId, setSessionId] = useState(0)
  const [state, setState] = useState<TerminalState>('idle')
  const [notice, setNotice] = useState<string | null>(null)
  const socketRef = useRef<WebSocket | null>(null)

  const start = useCallback(() => {
    setNotice(null)
    setSessionId((current) => current + 1)
  }, [])

  const stop = useCallback(() => {
    socketRef.current?.close(1000, 'closed by the operator')
  }, [])

  useEffect(() => {
    const container = containerRef.current
    if (sessionId === 0 || !container) return

    const term = new Terminal({
      convertEol: false,
      cursorBlink: true,
      fontFamily:
        'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
      fontSize: 13,
      theme: prefersDarkTerminal() ? DARK_THEME : LIGHT_THEME,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(container)
    fit.fit()
    terminalRef.current = term

    setState('connecting')
    const socket = new WebSocket(
      terminalSocketUrl(
        name,
        { rows: term.rows, cols: term.cols },
        window.location,
      ),
    )
    socket.binaryType = 'arraybuffer'
    socketRef.current = socket

    const encoder = new TextEncoder()
    const sendInput = (data: string) => {
      if (socket.readyState === WebSocket.OPEN) {
        socket.send(encoder.encode(data))
      }
    }

    socket.onopen = () => {
      setState('open')
      term.focus()
    }
    socket.onmessage = (event: MessageEvent<ArrayBuffer | string>) => {
      if (typeof event.data === 'string') {
        const parsed = parseTerminalEvent(event.data)
        if (parsed) setNotice(describeTerminalExit(parsed))
        return
      }
      term.write(new Uint8Array(event.data))
    }
    socket.onerror = () => {
      setNotice((current) => current ?? 'The terminal connection failed.')
    }
    socket.onclose = () => {
      setState('closed')
      setNotice((current) => current ?? 'Session ended.')
    }

    const dataSub = term.onData(sendInput)
    // xterm emits onBinary for input it has already encoded as latin-1
    // (mouse reports, some paste paths), which must not be re-encoded.
    const binarySub = term.onBinary((data) => {
      if (socket.readyState !== WebSocket.OPEN) return
      const bytes = new Uint8Array(data.length)
      for (let i = 0; i < data.length; i += 1)
        bytes[i] = data.charCodeAt(i) & 0xff
      socket.send(bytes)
    })
    const resizeSub = term.onResize(({ rows, cols }) => {
      if (socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ type: 'resize', rows, cols }))
      }
    })

    const observer = new ResizeObserver(() => {
      try {
        fit.fit()
      } catch {
        // fit throws while the container is detached or zero-sized
        // (a collapsed sidebar, a tab mid-transition); the next
        // observation re-fits once it has real dimensions again.
      }
    })
    observer.observe(container)

    return () => {
      observer.disconnect()
      dataSub.dispose()
      binarySub.dispose()
      resizeSub.dispose()
      socket.onclose = null
      socket.close(1000, 'terminal closed')
      socketRef.current = null
      term.dispose()
      terminalRef.current = null
    }
  }, [name, sessionId])

  // Theme changes retint the live terminal instead of restarting the
  // session, which would kill the operator's shell over a color change.
  useEffect(() => {
    const root = document.documentElement
    const apply = () => {
      const term = terminalRef.current
      if (term) {
        term.options.theme = prefersDarkTerminal() ? DARK_THEME : LIGHT_THEME
      }
    }
    const observer = new MutationObserver(apply)
    observer.observe(root, { attributes: true, attributeFilter: ['class'] })
    return () => observer.disconnect()
  }, [])

  const connected = state === 'open'

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <TerminalWindowIcon className="size-4" />
          Interactive terminal
        </CardTitle>
        <CardDescription>
          A real shell inside this app&apos;s running container, with a PTY,
          working arrow keys and Ctrl-C, and a terminal that resizes with this
          panel. Closing this page ends the shell on the node.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={connected ? 'success' : 'muted'}>
            {STATE_LABEL[state]}
          </Badge>
          {connected ? (
            <Button type="button" variant="outline" size="sm" onClick={stop}>
              <PlugsIcon className="size-3.5" data-icon="inline-start" />
              Disconnect
            </Button>
          ) : (
            <Button type="button" size="sm" onClick={start}>
              <PlugsConnectedIcon
                className="size-3.5"
                data-icon="inline-start"
              />
              {sessionId === 0 ? 'Start session' : 'Reconnect'}
            </Button>
          )}
          {notice ? (
            <span className="text-xs text-muted-foreground">{notice}</span>
          ) : null}
        </div>

        <div
          aria-label={`Interactive terminal for ${name}`}
          className="h-96 w-full overflow-hidden rounded-md border bg-zinc-50 p-2 dark:bg-zinc-950"
        >
          {/* Never given React children: xterm owns this subtree once a
              session starts, and the two must not both manage it. */}
          <div
            ref={containerRef}
            className={sessionId === 0 ? 'hidden' : 'h-full w-full'}
          />
          {sessionId === 0 ? (
            <p className="p-4 text-sm text-muted-foreground">
              Start a session to open a shell in this container. A shell can
              read this app&apos;s secrets, so it needs the root ability.
            </p>
          ) : null}
        </div>
      </CardContent>
    </Card>
  )
}
