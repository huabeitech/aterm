import { t } from '../../../../../shared/i18n'
import { isValidHostAlias } from '../../../../../shared/hostAlias'
import { domain } from '../../../api/types'
import type { FormState, HostSortScope, ViewID } from '../../../components/common/types'

export function formFromHost(host: domain.Host): FormState {
  return {
    directoryID: host.directoryID,
    alias: host.alias,
    hostName: host.hostName,
    user: host.user,
    port: String(host.port || 22),
    identityFile: host.identityFile,
    keySource: host.identityFile.startsWith('aterm-key://') ? 'managed' : 'system',
    passwordAuth: host.passwordAuth,
    password: '',
    passwordState: host.passwordAuth ? 'saved' : 'none',
    proxyJump: host.proxyJump,
    jumpHostID: host.jumpHostID,
    forwardAgent: host.forwardAgent,
    compression: host.compression,
    serverAliveInterval: optionalNumberToString(host.serverAliveInterval),
    serverAliveCountMax: optionalNumberToString(host.serverAliveCountMax),
    connectTimeout: optionalNumberToString(host.connectTimeout),
    remark: host.remark,
    favorite: host.favorite
  }
}

export function formToInput(form: FormState): domain.HostInput {
  return {
    directoryID: form.directoryID,
    alias: form.alias,
    hostName: form.hostName,
    user: form.user,
    port: parseOptionalNumber(form.port, 22),
    identityFile: form.identityFile,
    passwordAuth: form.passwordAuth,
    password: form.password,
    proxyJump: form.proxyJump,
    jumpHostID: form.jumpHostID,
    forwardAgent: form.forwardAgent,
    compression: form.compression,
    serverAliveInterval: parseOptionalNumber(form.serverAliveInterval, 0),
    serverAliveCountMax: parseOptionalNumber(form.serverAliveCountMax, 0),
    connectTimeout: parseOptionalNumber(form.connectTimeout, 0),
    remark: form.remark,
    favorite: form.favorite
  }
}

export function hostToInput(host: domain.Host): domain.HostInput {
  const {
    id: _id,
    lastUsedAt: _lastUsedAt,
    sortOrders: _sortOrders,
    createdAt: _createdAt,
    updatedAt: _updatedAt,
    ...input
  } = host
  return { ...input, password: '' }
}

export function hostSortScope(
  activeView: ViewID,
  activeDirectoryID: string | null
): HostSortScope | null {
  if (activeView === 'all') return null
  if (activeView === 'favorites') return { type: 'favorites', id: '' }
  if (activeView === 'uncategorized') return { type: 'uncategorized', id: '' }
  if (activeView === 'directory' && activeDirectoryID)
    return { type: 'directory', id: activeDirectoryID }
  return null
}

export function compareHostsInScope(a: domain.Host, b: domain.Host, scope: HostSortScope) {
  const key = hostSortScopeKey(scope)
  const aOrder = a.sortOrders?.[key] ?? Number.MAX_SAFE_INTEGER
  const bOrder = b.sortOrders?.[key] ?? Number.MAX_SAFE_INTEGER
  return aOrder - bOrder || a.alias.localeCompare(b.alias)
}

export function hostSortScopeKey(scope: HostSortScope): string {
  return `${scope.type}:${scope.id}`
}

export function compareAllHosts(a: domain.Host, b: domain.Host) {
  if (a.favorite !== b.favorite) return a.favorite ? -1 : 1
  const recentDifference = timestampValue(b.lastUsedAt) - timestampValue(a.lastUsedAt)
  return recentDifference || a.alias.localeCompare(b.alias)
}

export function validateForm(form: FormState): Partial<Record<keyof FormState, string>> {
  const errors: Partial<Record<keyof FormState, string>> = {}
  const alias = form.alias.trim()
  const hostName = form.hostName.trim()
  const port = Number(form.port.trim() || '22')
  if (!alias) errors.alias = t('hosts:hostAliasIsRequired')
  else if (!isValidHostAlias(alias)) errors.alias = t('errors:hostAliasFormat')
  if (!hostName) errors.hostName = t('hosts:hostnameOrIpAddressIsRequired')
  if (!Number.isInteger(port) || port < 1 || port > 65535)
    errors.port = t('hosts:portMustBeBetween1And65535')
  if (!form.passwordAuth && form.keySource === 'managed' && !form.identityFile)
    errors.identityFile = t('hosts:managedSshKeyIsRequired')
  return errors
}

function parseOptionalNumber(value: string, fallback: number) {
  const trimmed = value.trim()
  if (!trimmed) return fallback
  const parsed = Number(trimmed)
  return Number.isFinite(parsed) ? parsed : fallback
}

export function timestampValue(value: string) {
  if (!value) return 0
  const timestamp = Date.parse(value)
  return Number.isFinite(timestamp) ? timestamp : 0
}

function optionalNumberToString(value: number) {
  return value > 0 ? String(value) : ''
}
