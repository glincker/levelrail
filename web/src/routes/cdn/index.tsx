import { createFileRoute } from '@tanstack/react-router'
import { CdnOverviewPage } from '../../components/cdn/CdnOverviewPage'

export const Route = createFileRoute('/cdn/')({ component: CdnOverviewPage })
