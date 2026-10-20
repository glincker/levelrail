import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { useBrand } from '../../hooks/useBrand'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import { StepUpDialog } from '../env/StepUpDialog'
import { Badge } from '@/components/ui/badge'
import { useAppListOptional } from '../../queries/apps'
import { useNodeListOptional } from '../../queries/nodes'
import { mountConsoleTerminal } from '../../lib/cliConsoleTerminal'
import { DIM, GREEN, RED, RESET } from '../../lib/cliConsoleTheme'
import { QuickActions, SelectedAppRow } from './QuickActions'
import {
  backspace,
  complete,
  deleteForward,
  EMPTY_LINE,
  insertText,
  moveCursor,
  pushHistory,
  type LineState,
} from '../../lib/cliConsoleLine'
import {
  CLI_CONSOLE_STEP_UP_APP,
  cliConsoleSocketUrl,
  fetchCliCommands,
  parseCliServerMessage,
  type CliClientMessage,
  type CliCommand,
} from '../../lib/cliConsoleProtocol'
import {
  EMPTY_SANITIZE_STATE,
  sanitizeTerminalOutput,
  sanitizeTerminalText,
  type SanitizeState,
} from '../../lib/terminalSanitize'

type ConsoleState = 'idle' | 'connecting' | 'open' | 'closed'

const HIDDEN_DISCONNECT_MS = 60_000
const PING_INTERVAL_MS = 25_000

export function CliConsole({ className }: { className?: string }) {
  const { t } = useTranslation()
  const brand = useBrand()
  const prompt = `${brand.BinaryName}> `
  const containerRef = useRef<HTMLDivElement | null>(null)
  const termRef = useRef<Terminal | null>(null)
  const socketRef = useRef<WebSocket | null>(null)
  const treeRef = useRef<CliCommand[]>([])
  const stateRef = useRef<ConsoleState>('idle')
  const lineRef = useRef<LineState>(EMPTY_LINE)
  const historyRef = useRef<string[]>([])
  const historyIndexRef = useRef(-1)
  const draftRef = useRef('')
  const runningRef = useRef(false)
  const atLineStartRef = useRef(true)
  const cursorRowRef = useRef(0)
  const sanitizeRef = useRef<Record<'stdout' | 'stderr', SanitizeState>>({
    stdout: EMPTY_SANITIZE_STATE,
    stderr: EMPTY_SANITIZE_STATE,
  })
  const [state, setStateValue] = useState<ConsoleState>('idle')
  const [running, setRunningValue] = useState(false)
  const [commands, setCommands] = useState<CliCommand[]>([])
  const [selectedApp, setSelectedApp] = useState('')
  const appsQuery = useAppListOptional()
  const nodesQuery = useNodeListOptional()
  const appNames = useMemo(
    () => (appsQuery.data ?? []).map((a) => a.name),
    [appsQuery.data],
  )
  const nodes = useMemo(
    () => (nodesQuery.data ?? []).map((n) => ({ id: n.id, name: n.name })),
    [nodesQuery.data],
  )
  const appNamesRef = useRef<string[]>([])
  useEffect(() => {
    appNamesRef.current = appNames
  }, [appNames])

  const setRunning = useCallback((next: boolean) => {
    runningRef.current = next
    setRunningValue(next)
  }, [])

  const setState = useCallback((next: ConsoleState) => {
    stateRef.current = next
    setStateValue(next)
  }, [])

  const write = useCallback((text: string) => {
    termRef.current?.write(text)
  }, [])

  const newline = useCallback(() => {
    if (!atLineStartRef.current) {
      write('\r\n')
      cursorRowRef.current = 0
    }
    atLineStartRef.current = true
  }, [write])

  const printLine = useCallback(
    (text: string, color = '') => {
      newline()
      write(`${color}${sanitizeTerminalText(text)}${color ? RESET : ''}\r\n`)
    },
    [newline, write],
  )

  // Wrapped input spans several rows: climb to the first row, clear down,
  // repaint, then park the cursor by row and column (CUB never wraps).
  const redraw = useCallback(() => {
    const { buffer, cursor } = lineRef.current
    const cols = Math.max(1, termRef.current?.cols ?? 80)
    const endPos = prompt.length + buffer.length
    const cursorPos = prompt.length + cursor
    const up = cursorRowRef.current
    const endRow = Math.floor(endPos / cols)
    const wrapFix = endPos > 0 && endPos % cols === 0 ? ' \r' : ''
    const rowsBack = Math.max(0, endRow - Math.floor(cursorPos / cols))
    const col = cursorPos % cols
    write(
      `${up > 0 ? `\x1b[${String(up)}A` : ''}\r\x1b[J${GREEN}${prompt}${RESET}${buffer}${wrapFix}` +
        `${rowsBack > 0 ? `\x1b[${String(rowsBack)}A` : ''}\r${col > 0 ? `\x1b[${String(col)}C` : ''}`,
    )
    cursorRowRef.current = Math.floor(cursorPos / cols)
    atLineStartRef.current = false
  }, [prompt, write])

  const showPrompt = useCallback(() => {
    lineRef.current = EMPTY_LINE
    historyIndexRef.current = -1
    newline()
    cursorRowRef.current = 0
    redraw()
  }, [newline, redraw])

  const send = useCallback((message: CliClientMessage) => {
    const socket = socketRef.current
    if (socket?.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify(message))
    }
  }, [])

  const endSession = useCallback(
    (message: string) => {
      const socket = socketRef.current
      socketRef.current = null
      if (socket) {
        socket.onclose = null
        socket.onmessage = null
        socket.close(1000, 'console closed')
      }
      setRunning(false)
      setState('closed')
      printLine(message, DIM)
    },
    [printLine, setRunning, setState],
  )

  const openSocket = useCallback(() => {
    const socket = new WebSocket(cliConsoleSocketUrl(window.location))
    socketRef.current = socket
    let ready = false
    socket.onmessage = (event: MessageEvent<unknown>) => {
      if (typeof event.data !== 'string') return
      const msg = parseCliServerMessage(event.data)
      if (!msg) return
      switch (msg.type) {
        case 'ready':
          ready = true
          setState('open')
          printLine(t('cliConsole.connected', { server: msg.server }), DIM)
          showPrompt()
          termRef.current?.focus()
          break
        case 'output': {
          const cleaned = sanitizeTerminalOutput(
            msg.data,
            sanitizeRef.current[msg.stream],
          )
          sanitizeRef.current[msg.stream] = cleaned.state
          const text = cleaned.text.replace(/\r?\n/g, '\r\n')
          write(msg.stream === 'stderr' ? `${RED}${text}${RESET}` : text)
          if (text.length > 0) atLineStartRef.current = text.endsWith('\n')
          break
        }
        case 'exit':
          setRunning(false)
          sanitizeRef.current = {
            stdout: EMPTY_SANITIZE_STATE,
            stderr: EMPTY_SANITIZE_STATE,
          }
          if (msg.code !== 0) {
            printLine(t('cliConsole.exitCode', { code: msg.code }), DIM)
          }
          showPrompt()
          break
        case 'idle_timeout':
          endSession(t('cliConsole.disconnected'))
          break
        case 'error':
          setRunning(false)
          printLine(msg.message || t('cliConsole.genericError'), RED)
          showPrompt()
          break
      }
    }
    socket.onclose = () => {
      if (socketRef.current !== socket) return
      socketRef.current = null
      endSession(
        ready ? t('cliConsole.disconnected') : t('cliConsole.connectFailed'),
      )
    }
  }, [endSession, printLine, setRunning, setState, showPrompt, t, write])

  const fail = useCallback(
    (error: unknown) => {
      const detail = error instanceof Error ? error.message : ''
      setState('closed')
      printLine(
        detail
          ? t('cliConsole.connectFailedWith', { message: detail })
          : t('cliConsole.connectFailed'),
        RED,
      )
      printLine(t('cliConsole.reconnectHint'), DIM)
    },
    [printLine, setState, t],
  )
  const failRef = useRef(fail)
  useEffect(() => {
    failRef.current = fail
  }, [fail])
  const onGateError = useCallback((error: unknown) => {
    failRef.current(error)
  }, [])
  const gate = useStepUpGate(onGateError)

  const connect = useCallback(() => {
    if (stateRef.current === 'connecting' || stateRef.current === 'open') return
    setState('connecting')
    printLine(t('cliConsole.connecting'), DIM)
    void gate.guard(
      { action: 'manage', app: CLI_CONSOLE_STEP_UP_APP },
      async () => {
        treeRef.current = await fetchCliCommands()
        setCommands(treeRef.current)
        openSocket()
      },
    )
  }, [gate, openSocket, printLine, setState, t])

  const printHelp = useCallback(
    (topic: string) => {
      const tree = treeRef.current
      if (topic) {
        const cmd = tree.find((c) => c.name === topic)
        if (!cmd) {
          printLine(t('cliConsole.unknownCommand', { name: topic }), RED)
          return
        }
        printLine(`${cmd.name}: ${cmd.subcommands.join(', ')}`)
        return
      }
      printLine(t('cliConsole.helpIntro'))
      for (const cmd of tree) printLine(`  ${cmd.name}`)
      printLine(t('cliConsole.helpLocal'), DIM)
    },
    [printLine, t],
  )

  const submit = useCallback(() => {
    const line = lineRef.current.buffer.trim()
    write('\r\n')
    atLineStartRef.current = true
    cursorRowRef.current = 0
    historyRef.current = pushHistory(historyRef.current, line)
    lineRef.current = EMPTY_LINE
    historyIndexRef.current = -1
    if (line === '') return showPrompt()
    if (line === 'clear') {
      write('\x1b[2J\x1b[3J\x1b[H')
      atLineStartRef.current = true
      return showPrompt()
    }
    if (line === 'help' || line.startsWith('help ')) {
      printHelp(line.slice(4).trim())
      return showPrompt()
    }
    setRunning(true)
    send({ type: 'run', line })
  }, [printHelp, send, setRunning, showPrompt, write])

  const browseHistory = useCallback(
    (direction: -1 | 1) => {
      const history = historyRef.current
      if (history.length === 0) return
      let index = historyIndexRef.current
      if (index === -1) {
        if (direction === 1) return
        draftRef.current = lineRef.current.buffer
        index = history.length
      }
      index += direction
      if (index >= history.length) {
        index = -1
        lineRef.current = insertText(EMPTY_LINE, draftRef.current)
      } else {
        index = Math.max(0, index)
        lineRef.current = insertText(EMPTY_LINE, history[index] ?? '')
      }
      historyIndexRef.current = index
      redraw()
    },
    [redraw],
  )

  const runLine = useCallback(
    (line: string) => {
      if (stateRef.current !== 'open' || runningRef.current) return
      lineRef.current = insertText(EMPTY_LINE, line)
      historyIndexRef.current = -1
      redraw()
      submit()
    },
    [redraw, submit],
  )

  const stopLine = useCallback(() => {
    if (!runningRef.current) return
    write('^C\r\n')
    atLineStartRef.current = true
    send({ type: 'cancel' })
  }, [send, write])

  const onData = useCallback(
    (data: string) => {
      const current = stateRef.current
      if (current !== 'open') {
        if (
          (current === 'idle' || current === 'closed') &&
          data.includes('\r')
        ) {
          connect()
        }
        return
      }
      if (runningRef.current) {
        if (data === '\x03') {
          write('^C\r\n')
          atLineStartRef.current = true
          send({ type: 'cancel' })
        }
        return
      }
      const line = lineRef.current
      switch (data) {
        case '\r':
          return submit()
        case '\x7f':
        case '\b':
          lineRef.current = backspace(line)
          return redraw()
        case '\x1b[3~':
          lineRef.current = deleteForward(line)
          return redraw()
        case '\x1b[D':
        case '\x02':
          lineRef.current = moveCursor(line, line.cursor - 1)
          return redraw()
        case '\x1b[C':
        case '\x06':
          lineRef.current = moveCursor(line, line.cursor + 1)
          return redraw()
        case '\x1b[H':
        case '\x1bOH':
        case '\x01':
          lineRef.current = moveCursor(line, 0)
          return redraw()
        case '\x1b[F':
        case '\x1bOF':
        case '\x05':
          lineRef.current = moveCursor(line, line.buffer.length)
          return redraw()
        case '\x1b[A':
          return browseHistory(-1)
        case '\x1b[B':
          return browseHistory(1)
        case '\x15':
          lineRef.current = EMPTY_LINE
          return redraw()
        case '\x03':
          write('^C')
          return showPrompt()
        case '\x0c':
          write('\x1b[2J\x1b[3J\x1b[H')
          atLineStartRef.current = true
          return showPrompt()
        case '\t': {
          const result = complete(line, treeRef.current)
          lineRef.current = result.line
          if (result.candidates.length > 0) {
            printLine(result.candidates.join('  '), DIM)
          }
          return redraw()
        }
      }
      if (data.startsWith('\x1b')) return
      const text = Array.from(data)
        .filter((c) => c >= ' ' && c !== '\x7f')
        .join('')
      if (text === '') return
      lineRef.current = insertText(line, text)
      historyIndexRef.current = -1
      redraw()
    },
    [
      browseHistory,
      connect,
      printLine,
      redraw,
      send,
      showPrompt,
      submit,
      write,
    ],
  )
  const onDataRef = useRef(onData)
  useEffect(() => {
    onDataRef.current = onData
  }, [onData])

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    const mounted = mountConsoleTerminal(container, {
      onData: (d) => {
        onDataRef.current(d)
      },
      appNames: () => appNamesRef.current,
      onSelectApp: setSelectedApp,
    })
    const { term } = mounted
    termRef.current = term
    if (stateRef.current === 'idle') {
      term.write(`${DIM}${t('cliConsole.pressEnter')}${RESET}\r\n`)
      atLineStartRef.current = true
    }
    return () => {
      const socket = socketRef.current
      socketRef.current = null
      if (socket) {
        socket.onclose = null
        socket.onmessage = null
        socket.close(1000, 'console closed')
      }
      mounted.dispose()
      termRef.current = null
    }
    // Mount-only: later renders reach the terminal through refs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    let hiddenTimer: number | undefined
    const onVisibility = () => {
      window.clearTimeout(hiddenTimer)
      if (document.hidden && stateRef.current === 'open') {
        hiddenTimer = window.setTimeout(() => {
          if (stateRef.current === 'open') {
            endSession(t('cliConsole.disconnected'))
          }
        }, HIDDEN_DISCONNECT_MS)
      }
    }
    const ping = window.setInterval(() => {
      if (runningRef.current) send({ type: 'ping' })
    }, PING_INTERVAL_MS)
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      window.clearTimeout(hiddenTimer)
      window.clearInterval(ping)
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [endSession, send, t])

  const badge =
    state === 'open'
      ? { variant: 'success' as const, label: t('cliConsole.state.open') }
      : { variant: 'muted' as const, label: t(`cliConsole.state.${state}`) }

  return (
    <div
      className={`@container flex min-h-0 flex-col gap-2 ${className ?? ''}`}
    >
      <div className="flex items-center gap-2">
        <Badge variant={badge.variant}>{badge.label}</Badge>
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-2 @md:flex-row">
        <QuickActions
          commands={commands}
          appNames={appNames}
          nodes={nodes}
          selectedApp={selectedApp}
          disabled={state !== 'open'}
          running={running}
          onRun={runLine}
          onStop={stopLine}
          className="@md:w-52 @md:shrink-0 @md:overflow-y-auto"
        />
        <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-2">
          <SelectedAppRow
            appName={selectedApp}
            onClear={() => {
              setSelectedApp('')
            }}
          />
          <div
            aria-label={t('cliConsole.terminalLabel')}
            className="min-h-0 flex-1 overflow-hidden rounded-md border bg-zinc-50 p-2 dark:bg-zinc-950"
          >
            <div ref={containerRef} className="h-full w-full" />
          </div>
        </div>
      </div>
      <StepUpDialog
        scope={gate.pending?.scope ?? null}
        onVerified={() => {
          void gate.onVerified()
        }}
        onCancel={() => {
          gate.cancel()
          setState('closed')
          printLine(t('cliConsole.stepUpCancelled'), DIM)
          printLine(t('cliConsole.reconnectHint'), DIM)
        }}
      />
    </div>
  )
}
