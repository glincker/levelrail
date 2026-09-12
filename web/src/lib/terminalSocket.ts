// Wire helpers for GET /api/v1/apps/{name}/terminal
// (internal/api/terminal.go): the one WebSocket in this frontend, and a
// deliberate exception to the SSE-everywhere rule rather than a drift
// from it. SSE cannot carry keystrokes or a resize signal upward, and a
// terminal needs both on the same stream as its output. Log tailing and
// deploy progress stay on SSE (hooks/useLogStream.ts).
//
// Binary frames in both directions are raw terminal bytes; text frames
// are JSON control messages (client: resize; server: how the session
// ended).

/** Terminal size in character cells, matching the server's own query params. */
export interface TerminalSize {
  rows: number
  cols: number
}

/** The client's only control message today. */
export interface TerminalResizeMessage extends TerminalSize {
  type: 'resize'
}

/** The server's final text frame. */
export interface TerminalExitEvent {
  type: 'exit'
  exit_code?: number
  message?: string
}

/**
 * Builds the WebSocket URL for one app's terminal, relative to the page
 * this runs on, so the session cookie the endpoint authenticates with is
 * sent automatically.
 */
export function terminalSocketUrl(
  name: string,
  size: TerminalSize,
  location: Pick<Location, 'protocol' | 'host'>,
): string {
  const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const query = new URLSearchParams({
    rows: String(size.rows),
    cols: String(size.cols),
  })
  return `${scheme}//${location.host}/api/v1/apps/${encodeURIComponent(name)}/terminal?${query.toString()}`
}

/**
 * Reads the server's text frame, returning null for anything that is not
 * a recognizable exit event rather than throwing into the socket's
 * message handler.
 */
export function parseTerminalEvent(data: string): TerminalExitEvent | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(data)
  } catch {
    return null
  }
  if (typeof parsed !== 'object' || parsed === null) return null

  const candidate = parsed as Record<string, unknown>
  if (candidate.type !== 'exit') return null
  return {
    type: 'exit',
    exit_code:
      typeof candidate.exit_code === 'number' ? candidate.exit_code : undefined,
    message:
      typeof candidate.message === 'string' ? candidate.message : undefined,
  }
}

/** Human-readable summary of how a session ended, for the status line. */
export function describeTerminalExit(event: TerminalExitEvent): string {
  if (event.message) return event.message
  if (event.exit_code === undefined || event.exit_code === 0) {
    return 'Session ended.'
  }
  return `Session ended with exit code ${event.exit_code}.`
}
