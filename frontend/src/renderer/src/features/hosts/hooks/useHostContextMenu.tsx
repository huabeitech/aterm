import { t, i18n } from '../../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { useEffect, useMemo, useState } from 'react'
import type { ActionItem } from '@/components/ui/desktop'
import { Copy, Edit3, Eye, FolderOpen, Trash2, Zap } from 'lucide-react'
import { domain } from '../../../api/types'

type State = { host: domain.Host; ids: string[]; x: number; y: number }
type Options = {
  connectionFeaturesAvailable: boolean
  fileFeaturesAvailable?: boolean
  onOpenHost: (id: string) => void
  onEditHost: (id: string) => void
  onConnectHost: (id: string) => void
  onOpenFiles: (id: string) => void
  onCopySSHCommand: (id: string) => void
  onDeleteHost: (id: string) => void
  onDeleteHosts: (ids: string[]) => void
}

export function useHostContextMenu(options: Options) {
  useTranslation()
  const [contextMenu, setContextMenu] = useState<State | null>(null)

  useEffect(() => {
    if (!contextMenu) return
    const close = (event: PointerEvent) => {
      if (event.button === 2) return
      if (
        event.target instanceof Element &&
        event.target.closest('[data-slot=dropdown-menu-content]')
      )
        return
      setContextMenu(null)
    }
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setContextMenu(null)
    }
    document.addEventListener('pointerdown', close)
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('pointerdown', close)
      document.removeEventListener('keydown', closeOnEscape)
    }
  }, [contextMenu])

  const items = useMemo<ActionItem[]>(() => {
    if (!contextMenu) return []
    if (contextMenu.ids.length > 1)
      return [
        {
          key: 'delete-selected',
          danger: true,
          icon: <Trash2 size={14} />,
          label: t('hosts:deleteHosts', { count: contextMenu.ids.length })
        }
      ]
    const { connectionFeaturesAvailable } = options
    return [
      ...(connectionFeaturesAvailable
        ? [{ key: 'connect', icon: <Zap size={14} />, label: t('hosts:connect') }]
        : []),
      ...(options.fileFeaturesAvailable
        ? [{ key: 'files', icon: <FolderOpen size={14} />, label: t('hosts:openSftp') }]
        : []),
      { key: 'open', icon: <Eye size={14} />, label: t('hosts:details') },
      { key: 'edit', icon: <Edit3 size={14} />, label: t('hosts:edit') },
      { key: 'copy-ssh-command', icon: <Copy size={14} />, label: t('hosts:copySshCommand') },
      { type: 'divider' },
      { key: 'delete', danger: true, icon: <Trash2 size={14} />, label: t('hosts:delete') }
    ]
  }, [
    contextMenu,
    options.connectionFeaturesAvailable,
    options.fileFeaturesAvailable,
    i18n.language
  ])

  function handleClick({ key }: { key: string }) {
    if (!contextMenu) return
    const { host, ids } = contextMenu
    setContextMenu(null)
    if (key === 'open') options.onOpenHost(host.id)
    if (key === 'edit') options.onEditHost(host.id)
    if (key === 'connect') options.onConnectHost(host.id)
    if (key === 'files') options.onOpenFiles(host.id)
    if (key === 'copy-ssh-command') options.onCopySSHCommand(host.id)
    if (key === 'delete') options.onDeleteHost(host.id)
    if (key === 'delete-selected') options.onDeleteHosts(ids)
  }

  return {
    contextMenu,
    setContextMenu,
    contextMenuItems: items,
    handleContextMenuClick: handleClick
  }
}
