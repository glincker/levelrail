import { Link, useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  BookOpenIcon,
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
import { helpDocForPath } from './shell/contextHelp'

// Header-level help entry point, next to NotificationBell/ThemeToggle.
// Documentation and Troubleshooting go to the bundled in-app /help
// pages (work offline); Report an issue and Ask the community are
// external and only shown when brand configures a URL for them, the
// same "no invented link" rule HelpLink follows.
export function HelpMenu() {
  const brand = useBrand()
  const { t } = useTranslation()
  const contextDoc = useRouterState({
    select: (st) => helpDocForPath(st.location.pathname),
  })

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            type="button"
            variant="outline"
            size="icon-sm"
            aria-label={t('shell.help.label')}
          />
        }
      >
        <QuestionIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent>
        {contextDoc ? (
          <DropdownMenuItem
            render={<Link to="/help/$" params={{ _splat: contextDoc }} />}
          >
            <BookOpenIcon />
            {t('shell.help.forPage')}
          </DropdownMenuItem>
        ) : null}
        <DropdownMenuItem render={<Link to="/help" />}>
          <LifebuoyIcon />
          {t('shell.help.docs')}
        </DropdownMenuItem>
        <DropdownMenuItem
          render={<Link to="/help/$" params={{ _splat: 'troubleshooting' }} />}
        >
          <QuestionIcon />
          {t('shell.help.troubleshooting')}
        </DropdownMenuItem>
        <DropdownMenuItem render={<Link to="/settings/system-status" />}>
          <HeartbeatIcon />
          {t('shell.help.systemStatus')}
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
            {t('shell.help.report')}
          </DropdownMenuLinkItem>
        ) : null}
        {brand.DiscussionsURL ? (
          <DropdownMenuLinkItem
            href={brand.DiscussionsURL}
            target="_blank"
            rel="noreferrer"
          >
            <ChatCircleTextIcon />
            {t('shell.help.community')}
          </DropdownMenuLinkItem>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
