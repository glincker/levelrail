import { createFileRoute } from '@tanstack/react-router'
import { CachesPage } from '../../components/cdn/CachesPage'

export const Route = createFileRoute('/cdn/caches')({ component: CachesPage })
