import { Link } from '@tanstack/react-router'
import { CircleAlertGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { TONE } from '@/components/kit/tone'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { DashboardNotice } from '../../hooks/useDashboardNotices'

export function NoticeRows({ notices }: { notices: DashboardNotice[] }) {
  return (
    <ul className="divide-y divide-border">
      {notices.map((n) => {
        const Icon =
          n.tone === 'danger' ? CircleAlertGlassIcon : TriangleAlertGlassIcon
        return (
          <li
            key={n.id}
            className="flex flex-wrap items-center gap-x-3 gap-y-1 py-2 first:pt-0 last:pb-0"
          >
            <Icon
              aria-hidden="true"
              className={cn('size-5 shrink-0', TONE[n.tone].text)}
            />
            <span className="min-w-0 flex-1 text-sm">
              <span className="font-medium text-foreground">{n.title}</span>
              <span className="ml-2 text-muted-foreground">{n.detail}</span>
            </span>
            <Button size="sm" variant="outline" render={<Link to={n.to} />}>
              {n.action}
            </Button>
          </li>
        )
      })}
    </ul>
  )
}
