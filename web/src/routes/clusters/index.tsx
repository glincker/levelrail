import { createFileRoute } from '@tanstack/react-router'
import { ClustersPage } from '../../components/clusters/ClustersPage'

export const Route = createFileRoute('/clusters/')({
  component: ClustersPage,
})
