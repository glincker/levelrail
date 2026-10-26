// Wire types for /api/v1/stack-templates (internal/api/stack_templates.go).

import type { IacIssue } from '../queries/iac'

export interface StackTemplateVariable {
  name: string
  default: string
  description: string
}

export interface StackTemplateFile {
  name: string
  content: string
}

export interface StackTemplateSummary {
  id: string
  title: string
  description: string
  variables: StackTemplateVariable[]
  resource_kinds: string[]
  secret_names: string[]
}

export interface StackTemplateDetail extends StackTemplateSummary {
  files: StackTemplateFile[]
}

export interface StackTemplateRender {
  id: string
  files: StackTemplateFile[]
  issues?: IacIssue[]
}
