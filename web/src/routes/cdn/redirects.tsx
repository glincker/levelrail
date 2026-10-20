import { createFileRoute } from '@tanstack/react-router'
import { CdnControlsPage } from '../../components/cdn/CdnControlsPage'

export const Route = createFileRoute('/cdn/redirects')({
  component: () => <CdnControlsPage kind="redirects" />,
})
