import { Link } from '@tanstack/react-router'
import {
  ChatCircleTextIcon,
  GithubLogoIcon,
  HeartbeatIcon,
  LifebuoyIcon,
  QuestionIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLinkItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useBrand } from '../hooks/useBrand'

// Header-level help entry point, next to NotificationBell/ThemeToggle.
// Documentation and Troubleshooting go to the bundled in-app /help
// pages (work offline); Report an issue and Ask the community are
// external and only shown when brand configures a URL for them, the
// same "no invented link" rule HelpLink follows.
export function HelpMenu() {
  const brand = useBrand()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            type="button"
            variant="outline"
            size="icon-sm"
            aria-label="Help"
          />
        }
      >
        <QuestionIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent>
        <DropdownMenuItem render={<Link to="/help" />}>
          <LifebuoyIcon />
          Documentation
        </DropdownMenuItem>
        <DropdownMenuItem
          render={<Link to="/help/$" params={{ _splat: 'troubleshooting' }} />}
        >
          <QuestionIcon />
          Troubleshooting
        </DropdownMenuItem>
        <DropdownMenuItem render={<Link to="/settings/system-status" />}>
          <HeartbeatIcon />
          System status
        </DropdownMenuItem>
        {brand.SupportURL || brand.DiscussionsURL ? (
          <DropdownMenuSeparator />
        ) : null}
        {brand.SupportURL ? (
          <DropdownMenuLinkItem
            href={brand.SupportURL}
            target="_blank"
            rel="noreferrer"
          >
            <GithubLogoIcon />
            Report an issue
          </DropdownMenuLinkItem>
        ) : null}
        {brand.DiscussionsURL ? (
          <DropdownMenuLinkItem
            href={brand.DiscussionsURL}
            target="_blank"
            rel="noreferrer"
          >
            <ChatCircleTextIcon />
            Ask the community
          </DropdownMenuLinkItem>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
