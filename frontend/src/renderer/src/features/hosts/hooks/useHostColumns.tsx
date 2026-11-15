import { t, i18n } from '../../../../../shared/i18n'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useEffect, useMemo, useState } from 'react'
import { IconButton, Tip } from '@/components/ui/desktop'
import { toast } from '@/components/ui/toast'
import type { ReactNode } from 'react'
type HostColumn = {
  title: string
  key: string
  dataIndex?: keyof domain.Host
  width: number
  ellipsis?: boolean
  align?: 'right'
  hideInSetting?: boolean
  onHeaderCell?: () => { width: number; minWidth: number; onResize: (width: number) => void }
  render?: (value: unknown, host: domain.Host) => ReactNode
}
import { Check, Copy, Pin } from 'lucide-react'
import { ClipboardSetText } from '../../../api/runtime'
import { domain } from '../../../api/types'
import type { BusyAction } from '../../../components/common/types'

type ColumnKey =
  | 'alias'
  | 'directory'
  | 'user'
  | 'hostName'
  | 'port'
  | 'auth'
  | 'identityFile'
  | 'jumpHost'
  | 'forwardAgent'
  | 'compression'
  | 'serverAliveInterval'
  | 'serverAliveCountMax'
  | 'connectTimeout'
  | 'favorite'
  | 'remark'
  | 'lastUsedAt'
  | 'createdAt'
  | 'updatedAt'
const defaultWidths: Record<ColumnKey, number> = {
  alias: 190,
  directory: 180,
  user: 120,
  hostName: 180,
  port: 80,
  auth: 120,
  identityFile: 220,
  jumpHost: 150,
  forwardAgent: 170,
  compression: 120,
  serverAliveInterval: 180,
  serverAliveCountMax: 240,
  connectTimeout: 190,
  favorite: 90,
  remark: 220,
  lastUsedAt: 170,
  createdAt: 170,
  updatedAt: 170
}

export function useHostColumns({
  directories,
  busyAction,
  onCopySSHCommand
}: {
  directories: { id: string; label: string }[]
  busyAction: BusyAction
  onCopySSHCommand: (id: string) => Promise<boolean>
}) {
  useTranslation()
  const [columnWidths, setColumnWidths] = useState(defaultWidths)
  const resize = (key: ColumnKey, width: number) =>
    setColumnWidths((current) => ({ ...current, [key]: width }))
  const directoryByID = useMemo(
    () => new Map(directories.map((directory) => [directory.id, directory.label])),
    [directories]
  )
  const columns = useMemo<HostColumn[]>(
    () => [
      {
        title: t('hosts:hostAlias'),
        dataIndex: 'alias',
        key: 'alias',
        width: columnWidths.alias,
        ellipsis: true,
        onHeaderCell: () => ({
          width: columnWidths.alias,
          minWidth: 140,
          onResize: (width: number) => resize('alias', width)
        }),
        render: (_: unknown, host) => (
          <div className="host-name-cell grid grid-cols-[minmax(0,_1fr)_auto] gap-1 items-center min-w-0">
            <div className="host-name-content grid min-w-0">
              <span className="host-name-label inline-flex items-center gap-[5px] min-w-0 max-w-full">
                <span className="min-w-0 truncate text-sm font-normal" title={host.alias}>
                  {host.alias}
                </span>
                {host.favorite && (
                  <Pin
                    className="host-favorite-icon flex-none text-muted-foreground"
                    size={12}
                    fill="currentColor"
                    aria-label={t('hosts:favorite')}
                  />
                )}
              </span>
            </div>
            <CopySSHCommandButton host={host} disabled={!!busyAction} onCopy={onCopySSHCommand} />
          </div>
        )
      },
      {
        title: t('hosts:directory'),
        key: 'directory',
        width: columnWidths.directory,
        ellipsis: true,
        onHeaderCell: () => ({
          width: columnWidths.directory,
          minWidth: 130,
          onResize: (width: number) => resize('directory', width)
        }),
        render: (_, host) => directoryLabel(host.directoryID, directoryByID)
      },
      {
        title: t('hosts:username'),
        dataIndex: 'user',
        key: 'user',
        width: columnWidths.user,
        ellipsis: true,
        onHeaderCell: () => ({
          width: columnWidths.user,
          minWidth: 90,
          onResize: (width: number) => resize('user', width)
        }),
        render: (_, host) => host.user || '-'
      },
      {
        title: t('hosts:address'),
        dataIndex: 'hostName',
        key: 'hostName',
        width: columnWidths.hostName,
        ellipsis: true,
        onHeaderCell: () => ({
          width: columnWidths.hostName,
          minWidth: 130,
          onResize: (width: number) => resize('hostName', width)
        }),
        render: (_, host) => <HostNameCell value={host.hostName} />
      },
      {
        title: t('hosts:port'),
        dataIndex: 'port',
        key: 'port',
        width: columnWidths.port,
        align: 'right',
        onHeaderCell: () => ({
          width: columnWidths.port,
          minWidth: 70,
          onResize: (width: number) => resize('port', width)
        })
      },
      {
        title: t('hosts:authentication'),
        key: 'auth',
        width: columnWidths.auth,
        onHeaderCell: () => ({
          width: columnWidths.auth,
          minWidth: 110,
          onResize: (width: number) => resize('auth', width)
        }),
        render: (_, host) => (host.passwordAuth ? t('hosts:password') : t('hosts:publicKey'))
      },
      {
        title: t('hosts:privateKeyFile'),
        dataIndex: 'identityFile',
        key: 'identityFile',
        width: columnWidths.identityFile,
        ellipsis: true,
        onHeaderCell: () => ({
          width: columnWidths.identityFile,
          minWidth: 150,
          onResize: (width: number) => resize('identityFile', width)
        }),
        render: (_, host) => host.identityFile || t('hosts:defaultSshKeys')
      },
      {
        title: t('hosts:jumpHost'),
        key: 'jumpHost',
        width: columnWidths.jumpHost,
        ellipsis: true,
        onHeaderCell: () => ({
          width: columnWidths.jumpHost,
          minWidth: 120,
          onResize: (width: number) => resize('jumpHost', width)
        }),
        render: (_, host) => host.proxyJump || t('hosts:directConnection')
      },
      {
        title: t('hosts:agentForwarding'),
        dataIndex: 'forwardAgent',
        key: 'forwardAgent',
        width: columnWidths.forwardAgent,
        onHeaderCell: () => ({
          width: columnWidths.forwardAgent,
          minWidth: 110,
          onResize: (width: number) => resize('forwardAgent', width)
        }),
        render: (_, host) => yesNo(host.forwardAgent)
      },
      {
        title: t('hosts:compression'),
        dataIndex: 'compression',
        key: 'compression',
        width: columnWidths.compression,
        onHeaderCell: () => ({
          width: columnWidths.compression,
          minWidth: 110,
          onResize: (width: number) => resize('compression', width)
        }),
        render: (_, host) => yesNo(host.compression)
      },
      {
        title: t('hosts:keepaliveInterval'),
        dataIndex: 'serverAliveInterval',
        key: 'serverAliveInterval',
        width: columnWidths.serverAliveInterval,
        align: 'right',
        onHeaderCell: () => ({
          width: columnWidths.serverAliveInterval,
          minWidth: 120,
          onResize: (width: number) => resize('serverAliveInterval', width)
        }),
        render: (_, host) => secondsLabel(host.serverAliveInterval)
      },
      {
        title: t('hosts:maxUnansweredKeepalives'),
        dataIndex: 'serverAliveCountMax',
        key: 'serverAliveCountMax',
        width: columnWidths.serverAliveCountMax,
        align: 'right',
        onHeaderCell: () => ({
          width: columnWidths.serverAliveCountMax,
          minWidth: 110,
          onResize: (width: number) => resize('serverAliveCountMax', width)
        }),
        render: (_, host) => host.serverAliveCountMax || '-'
      },
      {
        title: t('hosts:connectionTimeout'),
        dataIndex: 'connectTimeout',
        key: 'connectTimeout',
        width: columnWidths.connectTimeout,
        align: 'right',
        onHeaderCell: () => ({
          width: columnWidths.connectTimeout,
          minWidth: 100,
          onResize: (width: number) => resize('connectTimeout', width)
        }),
        render: (_, host) => secondsLabel(host.connectTimeout)
      },
      {
        title: t('hosts:favorite'),
        dataIndex: 'favorite',
        key: 'favorite',
        width: columnWidths.favorite,
        onHeaderCell: () => ({
          width: columnWidths.favorite,
          minWidth: 80,
          onResize: (width: number) => resize('favorite', width)
        }),
        render: (_, host) => yesNo(host.favorite)
      },
      {
        title: t('hosts:notes'),
        dataIndex: 'remark',
        key: 'remark',
        width: columnWidths.remark,
        ellipsis: true,
        onHeaderCell: () => ({
          width: columnWidths.remark,
          minWidth: 140,
          onResize: (width: number) => resize('remark', width)
        }),
        render: (_, host) => (
          <span className="host-remark-cell block max-w-full overflow-hidden text-ellipsis whitespace-nowrap">
            {host.remark || '-'}
          </span>
        )
      },
      {
        title: t('hosts:lastUsed'),
        dataIndex: 'lastUsedAt',
        key: 'lastUsedAt',
        width: columnWidths.lastUsedAt,
        onHeaderCell: () => ({
          width: columnWidths.lastUsedAt,
          minWidth: 140,
          onResize: (width: number) => resize('lastUsedAt', width)
        }),
        render: (_, host) => formatDateTime(host.lastUsedAt)
      },
      {
        title: t('hosts:created'),
        dataIndex: 'createdAt',
        key: 'createdAt',
        width: columnWidths.createdAt,
        onHeaderCell: () => ({
          width: columnWidths.createdAt,
          minWidth: 140,
          onResize: (width: number) => resize('createdAt', width)
        }),
        render: (_, host) => formatDateTime(host.createdAt)
      },
      {
        title: t('hosts:updated'),
        dataIndex: 'updatedAt',
        key: 'updatedAt',
        width: columnWidths.updatedAt,
        onHeaderCell: () => ({
          width: columnWidths.updatedAt,
          minWidth: 140,
          onResize: (width: number) => resize('updatedAt', width)
        }),
        render: (_, host) => formatDateTime(host.updatedAt)
      }
    ],
    [busyAction, columnWidths, directoryByID, onCopySSHCommand, i18n.language]
  )
  return { columns }
}

function HostNameCell({ value }: { value: string }) {
  useTranslation()
  return (
    <span className="host-address-cell flex w-full items-center gap-1 min-w-0">
      <span
        className="host-address-text min-w-0 flex-1 [.host-address-cell_&]:overflow-hidden [.host-address-cell_&]:text-ellipsis [.host-address-cell_&]:whitespace-nowrap"
        title={value}
      >
        {value || '-'}
      </span>
      {value && (
        <InlineCopyButton
          key={value}
          className="host-address-copy"
          title={t('hosts:copyAddress')}
          label={t('hosts:copyAddressValue', { address: value })}
          onCopy={async () => {
            try {
              await ClipboardSetText(value)
              toast.add({ title: t('hosts:hostnameCopied'), type: 'success' })
              return true
            } catch {
              toast.add({ title: t('hosts:unableToCopyHostname'), type: 'error' })
              return false
            }
          }}
        />
      )}
    </span>
  )
}

function directoryLabel(directoryID: string, directoryByID: Map<string, string>) {
  if (!directoryID) return t('hosts:uncategorized')
  return directoryByID.get(directoryID) || t('hosts:missingDirectory')
}

function yesNo(value: boolean) {
  return value ? t('hosts:yes') : t('hosts:no')
}

function secondsLabel(value: number) {
  return value > 0 ? `${value}s` : '-'
}

function formatDateTime(value: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString(i18n.language)
}

function CopySSHCommandButton({
  host,
  disabled,
  onCopy
}: {
  host: domain.Host
  disabled: boolean
  onCopy: (id: string) => Promise<boolean>
}) {
  useTranslation()
  return (
    <InlineCopyButton
      className="host-command-copy"
      title={t('hosts:copySshCommand2')}
      label={t('hosts:copyHostCommand', { host: host.alias })}
      disabled={disabled}
      onCopy={() => onCopy(host.id)}
    />
  )
}

function InlineCopyButton({
  className,
  title,
  label,
  disabled = false,
  onCopy
}: {
  className: string
  title: string
  label: string
  disabled?: boolean
  onCopy: () => Promise<boolean>
}) {
  useTranslation()
  const [copied, setCopied] = useState(false)
  const [copying, setCopying] = useState(false)
  useEffect(() => {
    if (!copied) return
    const timer = window.setTimeout(() => setCopied(false), 1800)
    return () => window.clearTimeout(timer)
  }, [copied])
  return (
    <Tip title={copied ? t('hosts:copied2') : title}>
      <IconButton
        className={cn(
          className,
          'size-6 shrink-0 opacity-0 pointer-events-none',
          '[.host-native-table_tr:hover_&]:opacity-100 [.host-native-table_tr:hover_&]:pointer-events-auto',
          '[.host-native-table_tr:focus-within_&]:opacity-100 [.host-native-table_tr:focus-within_&]:pointer-events-auto',
          copied && 'opacity-100 text-green-600'
        )}
        icon={copied ? <Check size={14} /> : <Copy size={14} />}
        aria-label={label}
        disabled={disabled || copying}
        draggable={false}
        onMouseDown={(event) => event.stopPropagation()}
        onDoubleClick={(event) => event.stopPropagation()}
        onDragStart={(event) => {
          event.preventDefault()
          event.stopPropagation()
        }}
        onClick={async (event) => {
          event.stopPropagation()
          setCopying(true)
          try {
            if (await onCopy()) setCopied(true)
          } finally {
            setCopying(false)
          }
        }}
      />
    </Tip>
  )
}
