import type {
  JsonValue,
  PluginCommand,
  PluginViewAction
} from '../../../../shared/contracts/plugins'

export interface OpenPluginView {
  invocationId: string
  id: string
  context?: JsonValue
}

/** View identity determines toggling; context only selects content within an open view. */
export function applyViewAction(
  current: OpenPluginView | undefined,
  action: PluginViewAction,
  invocationId: string
): OpenPluginView | undefined {
  const matches = current?.id === action.view
  if (action.type === 'view.close') return matches ? undefined : current
  if (action.type === 'view.toggle' && matches) return undefined
  return { id: action.view, context: action.context, invocationId }
}

export function isViewToggleSelected(command: PluginCommand | undefined, view?: OpenPluginView) {
  if (command?.action?.type !== 'view.toggle') return undefined
  return command.action.view === view?.id
}
