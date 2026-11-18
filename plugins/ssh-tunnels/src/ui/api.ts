import type { Tunnel, TunnelInput, TunnelStatus } from './types'
async function request<T>(op: string, id = '', input?: TunnelInput): Promise<T> {
  try {
    return (await atermView.executeCommand(
      op === 'start' || op === 'test' ? 'tunnels.connect' : 'tunnels.request',
      { op, id, ...(input ? { input: { ...input } } : {}) },
    )) as T
  } catch (error) {
    throw new Error(
      String(error instanceof Error ? error.message : error).replace(
        /^rpc error: code = \w+ desc = /,
        '',
      ),
    )
  }
}
export const desktop = {
  tunnels: {
    list: () => request<TunnelStatus[]>('list'),
    save: (id: string, input: TunnelInput) => request<Tunnel>('save', id, input),
    start: (id: string) => request<void>('start', id),
    stop: (id: string) => request<void>('stop', id),
    delete: (id: string) => request<void>('delete', id),
    test: (id: string, input: TunnelInput) =>
      request<{ localListener: 'checked' | 'existing' }>('test', id, input),
    onChanged: (callback: () => void) => atermView.onDidChange(callback),
  },
  library: { copyText: (text: string) => atermView.executeCommand('tunnels.copy', { text }) },
}
