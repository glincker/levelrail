// Wire types for container registry vendor presets and private package
// registries (internal/api/package_registries.go).

export interface RegistryVendor {
  id: string
  name: string
  default_host?: string
  host_hint?: string
  fixed_username?: string
  token_lifetime_hours?: number
  brand: string
}

export type PackageEcosystem =
  'npm' | 'pypi' | 'maven' | 'nuget' | 'cargo' | 'go' | 'rubygems'

export type PackageRegistryScope = 'instance' | 'project' | 'app'

export interface PackageRegistry {
  id: string
  name: string
  ecosystem: PackageEcosystem
  url: string
  username?: string
  module_patterns?: string
  scope_kind: PackageRegistryScope
  scope_ref?: string
  build_secret_id: string
  created_by?: string
  created_at: string
}

export interface CreatePackageRegistryRequest {
  name: string
  ecosystem: PackageEcosystem
  url: string
  username?: string
  secret: string
  module_patterns?: string
  scope_kind: PackageRegistryScope
  scope_ref?: string
}

export interface ConnectionTestResult {
  ok: boolean
  message: string
  tested_at: string
}

export interface RegistryCredentialUsage {
  services: string[]
}
