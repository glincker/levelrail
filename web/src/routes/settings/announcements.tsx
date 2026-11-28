import { createFileRoute } from '@tanstack/react-router'
import { AnnouncementsPage } from '@/components/announcements/AnnouncementsPage'

export const Route = createFileRoute('/settings/announcements')({
  component: AnnouncementsPage,
})
