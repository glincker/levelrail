import { createFileRoute } from '@tanstack/react-router'
import { ImagesPage } from '../components/images/ImagesPage'

export const Route = createFileRoute('/images')({
  component: ImagesPage,
})
