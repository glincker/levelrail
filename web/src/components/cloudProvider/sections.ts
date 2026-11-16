import type { ComponentType } from 'react'
import type {
  CloudProviderFeature,
  CloudProviderPresentation,
} from '../../lib/cloudProviderBrand'
import type { NodeProviderResource } from '../../types/nodeProvision'
import { ProviderConnectionSection } from './ProviderConnectionSection'
import { ProviderCapabilitiesPanel } from './ProviderCapabilitiesPanel'
import { ProviderResourcesSection } from './ProviderResourcesSection'
import { ProviderServersSection } from './ProviderServersSection'
import type { ProviderLabels } from './useProviderLabels'

export interface SectionProps {
  provider: CloudProviderPresentation
  resource: NodeProviderResource
  labels: ProviderLabels
}

// To add a section (discovery, spend, firewalls, volumes, DOKS): register its
// component here and list its feature id in the provider entries of
// lib/cloudProviderBrand.ts. A listed feature with no component here is skipped.
export const SECTIONS: Partial<
  Record<CloudProviderFeature, ComponentType<SectionProps>>
> = {
  connection: ProviderConnectionSection,
  capabilities: ProviderCapabilitiesPanel,
  servers: ProviderServersSection,
  resources: ProviderResourcesSection,
}

export const SECTION_ORDER: readonly CloudProviderFeature[] = [
  'connection',
  'capabilities',
  'servers',
  'resources',
]
