import type { FormSchema } from './formSnippet'
import type { NotificationChannel } from '../types/notificationChannel'

export type FormRecipeId = 'contact' | 'newsletter' | 'feedback'

export interface FormRecipe {
  id: FormRecipeId
  name: string
  schema: FormSchema
  // Channel kinds that satisfy "notify me here"; the first enabled match is wired up.
  notifyKinds: NotificationChannel['kind'][]
}

export const FORM_RECIPES: FormRecipe[] = [
  {
    id: 'contact',
    name: 'contact',
    notifyKinds: ['email', 'resend'],
    schema: {
      fields: [
        { name: 'name', type: 'text', label: 'Name', required: true },
        { name: 'email', type: 'email', label: 'Email', required: true },
        { name: 'message', type: 'textarea', label: 'Message', required: true },
      ],
      success_message: 'Thanks, we will reply by email soon.',
    },
  },
  {
    id: 'newsletter',
    name: 'newsletter',
    notifyKinds: ['email', 'resend'],
    schema: {
      fields: [
        { name: 'email', type: 'email', label: 'Email', required: true },
      ],
      success_message: 'You are on the list.',
    },
  },
  {
    id: 'feedback',
    name: 'feedback',
    notifyKinds: ['slack', 'mattermost', 'discord'],
    schema: {
      fields: [
        {
          name: 'message',
          type: 'textarea',
          label: 'Feedback',
          required: true,
        },
        { name: 'email', type: 'email', label: 'Email (optional)' },
      ],
      success_message: 'Thanks for the feedback.',
    },
  },
]

export function recipeChannelIds(
  recipe: FormRecipe,
  channels: NotificationChannel[],
): string[] {
  const hit = channels.find(
    (c) => c.enabled && recipe.notifyKinds.includes(c.kind),
  )
  return hit ? [hit.id] : []
}
