import { entries } from '../.vitepress/templates.mts'

export interface GalleryCard {
  id: string
  name: string
  slogan: string
  category: string
}

declare const data: GalleryCard[]
export { data }

export default {
  load(): GalleryCard[] {
    return entries
      .map(({ id, name, slogan, category }) => ({ id, name, slogan, category }))
      .sort((a, b) => a.name.localeCompare(b.name))
  },
}
