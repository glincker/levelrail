import { useTranslation } from 'react-i18next'
import {
  ArchiveGlassIcon,
  HardDriveUploadGlassIcon,
  InboxGlassIcon,
  QueueWorkerGlassIcon,
  UsersGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'

const PANELS = [
  { key: 'mailboxes', icon: <InboxGlassIcon size="1.2em" /> },
  { key: 'aliases', icon: <UsersGlassIcon size="1.2em" /> },
  { key: 'queue', icon: <QueueWorkerGlassIcon size="1.2em" /> },
  { key: 'storage', icon: <HardDriveUploadGlassIcon size="1.2em" /> },
  { key: 'backup', icon: <ArchiveGlassIcon size="1.2em" /> },
] as const

/** Placeholders that say what each panel will do. No data is shown until the APIs exist. */
export function MailSoonPanels() {
  const { t } = useTranslation('mailServer')
  return (
    <ul className="grid gap-2 sm:grid-cols-2 xl:grid-cols-5">
      {PANELS.map((p) => (
        <li
          key={p.key}
          className="rounded-lg border border-dashed border-border p-3 text-sm"
        >
          <div className="mb-1 flex items-center justify-between gap-2">
            <span className="flex items-center gap-1.5 font-medium">
              {p.icon}
              {t(`soon.${p.key}.title`)}
            </span>
            <Badge variant="outline">{t('soon.label')}</Badge>
          </div>
          <p className="text-xs text-muted-foreground">
            {t(`soon.${p.key}.body`)}
          </p>
        </li>
      ))}
    </ul>
  )
}
