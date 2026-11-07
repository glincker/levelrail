import { createFileRoute } from '@tanstack/react-router'
import { TagsManager } from '../components/tags/TagsManager'

export const Route = createFileRoute('/tags')({
  component: TagsManager,
})
