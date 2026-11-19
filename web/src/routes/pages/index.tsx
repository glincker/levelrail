import { createFileRoute } from '@tanstack/react-router'
import { PagesList } from '../../components/pages/PagesList'

export const Route = createFileRoute('/pages/')({ component: PagesList })
