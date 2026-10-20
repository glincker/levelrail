import {
  ClockCounterClockwiseIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useAiSessions, useDeleteAiSession } from '../queries/aiAssistant'
import { formatAge } from '../lib/format'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

// Lists every past session (GET /api/v1/ai/sessions) so one can be
// resumed or removed (DELETE .../sessions/{id}); the composer itself
// never browses history, it only ever has the one active session
// useAiChatSession owns. Suspense-backed (useAiSessions), so this must
// stay inside AiChatPanel's own boundary, never rendered standalone.
//
// Resume and delete are two separate menu items per session, not a
// button nested inside a menu item: every other DropdownMenuItem in
// this app (AppRowActions.tsx) is itself the clickable action, and
// nesting a second interactive control inside one would fight the
// menu's own click-to-select-and-close behavior.
export function AiChatHistoryMenu({
  activeSessionId,
  onResume,
}: Readonly<{
  activeSessionId: string | null
  onResume: (sessionId: string) => void
}>) {
  const { data: sessions } = useAiSessions()
  const deleteSession = useDeleteAiSession()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button type="button" variant="outline" size="sm" />}
      >
        <ClockCounterClockwiseIcon />
        History
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {sessions.length === 0 ? (
          <div className="px-2 py-1.5 text-sm text-muted-foreground">
            No previous sessions yet.
          </div>
        ) : (
          sessions.map((session, i) => (
            <div key={session.id}>
              {i > 0 ? <DropdownMenuSeparator /> : null}
              <DropdownMenuItem
                onClick={() => {
                  onResume(session.id)
                }}
              >
                <ClockCounterClockwiseIcon />
                <span className="truncate">
                  {session.id === activeSessionId
                    ? 'Current session'
                    : 'Resume session'}{' '}
                  <span className="text-muted-foreground">
                    · {formatAge(session.updated_at)}
                  </span>
                </span>
              </DropdownMenuItem>
              <DropdownMenuItem
                className="text-destructive [&_svg]:text-destructive"
                onClick={() => {
                  deleteSession.mutate(session.id)
                }}
              >
                <TrashIcon />
                Delete this session
              </DropdownMenuItem>
            </div>
          ))
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
