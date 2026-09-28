import { useState } from 'react'
import { GaugeIcon } from '@phosphor-icons/react/dist/ssr'
import { useQuery } from '@tanstack/react-query'
import { Suggestion } from '@/components/kit'
import { nodeDetailQueryOptions } from '../queries/nodes'
import type { NodeResource } from '../types/nodeDetail'
import { useBrand } from '../hooks/useBrand'
import { splitHash } from '../lib/docsPaths'
import docsPathIndex from 'virtual:docs-path-index'

const DISMISS_KEY = 'build-strain-suggestion-dismissed'

function readDismissed(): boolean {
  try {
    return window.sessionStorage.getItem(DISMISS_KEY) === '1'
  } catch {
    return false
  }
}

function buildNodeRoutingDocHref(docsUrl: string): string | null {
  const [basePath, hash] = splitHash('/build-node-routing')
  if (docsPathIndex.has(basePath)) return `/help${basePath}${hash}`
  if (docsUrl) return `${docsUrl}/build-node-routing`
  return null
}

// Reuses the same node_resource_usage/node_disk_space alert status
// GET /api/v1/nodes/{id} already computes (internal/alerting), rather
// than inventing a second threshold mechanism: a sole node running both
// the control plane and every build shares CPU and disk between the two,
// so sustained pressure there is exactly the signal that a second node
// (or, once one exists, marking a node build-only) would relieve.
// Renders nothing for a multi-node fleet: at that point the accepts
// build workloads toggle on each node's own page is the actual lever.
export function BuildStrainSuggestion({ nodes }: { nodes: NodeResource[] }) {
  const brand = useBrand()
  const [dismissed, setDismissed] = useState(readDismissed)
  const soleNode = nodes.length === 1 ? nodes[0] : undefined
  const detail = useQuery({
    ...nodeDetailQueryOptions(soleNode?.id ?? ''),
    enabled: Boolean(soleNode) && !dismissed,
    retry: false,
  })

  if (!soleNode || dismissed) return null
  const status = detail.data?.alert_status
  const strained =
    status?.node_resource_usage === 'firing' ||
    status?.node_disk_space === 'firing'
  if (!strained) return null

  const href = buildNodeRoutingDocHref(brand.DocsURL)

  return (
    <div className="mb-4">
      <Suggestion
        tone="warning"
        icon={<GaugeIcon />}
        title="This node is under sustained load"
        detail="Builds and apps both run on this one node, so a heavy build competes with whatever else is serving traffic. Add a second node to spread that load, then route builds to it from that node's detail page."
        actions={
          href
            ? [
                {
                  label: 'Read the build routing guide',
                  onClick: () => {
                    window.open(href, '_blank', 'noreferrer')
                  },
                },
              ]
            : []
        }
        onDismiss={() => {
          setDismissed(true)
          try {
            window.sessionStorage.setItem(DISMISS_KEY, '1')
          } catch {
            // Dismissal just will not persist across reloads.
          }
        }}
      />
    </div>
  )
}
