import * as React from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ArchiveGlassIcon,
  ExternalLinkGlassIcon,
  InboxGlassIcon,
  PlusGlassIcon,
} from '@thesvg/ui-react'
import { toast } from '@/components/ui/toast'
import { useCanWriteSensitive } from '../hooks/useCanWriteSensitive'
import { useExperimentalFeatures } from '../hooks/useExperimental'
import { appUrlsQueryOptions } from '../queries/appUrls'
import { useRunMailBackup } from '../queries/mailBackup'
import { isMailUp, useMailStatus } from '../queries/mailStatus'
import { webmailUrl } from './mailserver/mailSetup'
import type { PaletteItem } from './commandPaletteData'

export interface MailPaletteInput {
  /** Mail flag on and the mail app installed; otherwise no entries at all. */
  available: boolean
  up: boolean
  canWrite: boolean
  webmail: string | undefined
  labels: {
    add: string
    backup: string
    webmail: string
    queue: string
    keywords: string
  }
  go: (search: {
    tab: 'mailboxes' | 'overview'
    add?: true
    queue?: true
  }) => void
  backup: () => void
  openUrl: (url: string) => void
}

/** Pure builder so the gating rules are testable without a router. */
export function buildMailPaletteItems(i: MailPaletteInput): PaletteItem[] {
  if (!i.available) return []
  const item = (
    key: string,
    label: string,
    icon: React.ReactNode,
    run: () => void,
  ): PaletteItem => ({
    key,
    label,
    group: 'Actions',
    icon,
    keywords: i.labels.keywords,
    run,
  })
  const items: PaletteItem[] = []
  if (i.up && i.canWrite) {
    items.push(
      item(
        'action-mail-add',
        i.labels.add,
        <PlusGlassIcon size="1.4em" />,
        () => i.go({ tab: 'mailboxes', add: true }),
      ),
      item(
        'action-mail-backup',
        i.labels.backup,
        <ArchiveGlassIcon size="1.4em" />,
        i.backup,
      ),
    )
  }
  if (i.webmail) {
    const url = i.webmail
    items.push(
      item(
        'action-mail-webmail',
        i.labels.webmail,
        <ExternalLinkGlassIcon size="1.4em" />,
        () => i.openUrl(url),
      ),
    )
  }
  if (i.up) {
    items.push(
      item(
        'action-mail-queue',
        i.labels.queue,
        <InboxGlassIcon size="1.4em" />,
        () => i.go({ tab: 'overview', queue: true }),
      ),
    )
  }
  return items
}

const NONE: PaletteItem[] = []

/** Cmd+K mail actions; the status read is skipped entirely while the mail flag is off. */
export function useMailPaletteItems(): PaletteItem[] {
  const { t } = useTranslation('palette')
  const navigate = useNavigate()
  const flagOn = useExperimentalFeatures().includes('mail')
  const status = useMailStatus({ enabled: flagOn, poll: false }).data
  const canWrite = useCanWriteSensitive()
  const app = status?.installed ? status.app : undefined
  const urls = useQuery({
    ...appUrlsQueryOptions(app ?? ''),
    enabled: flagOn && !!app,
    refetchInterval: false,
  }).data
  const run = useRunMailBackup(app)
  const { mutate } = run
  const available = flagOn && !!status?.installed
  const up = isMailUp(status)
  const webmail = webmailUrl(urls)

  return React.useMemo(() => {
    if (!available) return NONE
    return buildMailPaletteItems({
      available,
      up,
      canWrite,
      webmail,
      labels: {
        add: t('mail.addMailbox'),
        backup: t('mail.backupNow'),
        webmail: t('mail.openWebmail'),
        queue: t('mail.openQueue'),
        keywords: t('mail.keywords'),
      },
      go: (search) => void navigate({ to: '/mail', search }),
      backup: () =>
        mutate(undefined, {
          onSuccess: () =>
            toast.add({ title: t('mail.backupStarted'), type: 'success' }),
          onError: (e) =>
            toast.add({
              title: e instanceof Error ? e.message : t('mail.backupFailed'),
              type: 'error',
            }),
        }),
      openUrl: (url) => window.open(url, '_blank', 'noopener,noreferrer'),
    })
  }, [available, up, canWrite, webmail, t, navigate, mutate])
}
