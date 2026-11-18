import { useState } from 'react'
import { domain } from '../../../api/types'
import {
  CreateHost,
  DeleteHost,
  DeleteHosts,
  ListHosts,
  MoveHosts,
  ReorderHosts,
  UpdateHost
} from '../../../api/hosts'
import type { HostSortScope } from '../../../components/common/types'
import { applyOptimisticHostDirectoryMove } from '../model/hostMove'

export function useHostRepository() {
  const [hosts, setHosts] = useState<domain.Host[]>([])
  const [selectedID, setSelectedID] = useState<string | null>(null)
  const [selectedHostIDs, setSelectedHostIDs] = useState<string[]>([])
  const [selectionAnchorID, setSelectionAnchorID] = useState<string | null>(null)
  const [draggingHostIDs, setDraggingHostIDs] = useState<string[]>([])

  async function loadHosts() {
    const next = await ListHosts()
    setHosts(next)
    return next
  }

  const createHost = (input: domain.HostInput) => CreateHost(input)
  const updateHost = (id: string, input: domain.HostInput) => UpdateHost(id, input)
  const deleteHost = (id: string) => DeleteHost(id)
  const deleteHosts = (ids: string[]) => DeleteHosts(ids)
  const moveHosts = (ids: string[], directoryID: string) => MoveHosts(ids, directoryID)
  const reorderHosts = (scope: HostSortScope, ids: string[]) =>
    ReorderHosts(scope.type, scope.id, ids)

  async function moveHostsOptimistically(ids: string[], directoryID: string) {
    const previous = hosts
    setHosts((current) => applyOptimisticHostDirectoryMove(current, ids, directoryID))
    try {
      return await moveHosts(ids, directoryID)
    } catch (error) {
      setHosts(previous)
      throw error
    }
  }

  async function reorderHostsOptimistically(scope: HostSortScope, ids: string[]) {
    const previous = hosts
    const scopeKey = `${scope.type}:${scope.id}`
    const orderByID = new Map(ids.map((id, index) => [id, index]))
    setHosts((current) =>
      current.map((host) =>
        orderByID.has(host.id)
          ? {
              ...host,
              sortOrders: {
                ...(host.sortOrders ?? {}),
                [scopeKey]: orderByID.get(host.id) ?? Number.MAX_SAFE_INTEGER
              }
            }
          : host
      )
    )
    try {
      const reordered = await reorderHosts(scope, ids)
      const byID = new Map(reordered.map((host) => [host.id, host]))
      setHosts((current) => current.map((host) => byID.get(host.id) ?? host))
      return reordered
    } catch (error) {
      setHosts(previous)
      throw error
    }
  }

  return {
    hosts,
    setHosts,
    selectedID,
    setSelectedID,
    selectedHostIDs,
    setSelectedHostIDs,
    selectionAnchorID,
    setSelectionAnchorID,
    draggingHostIDs,
    setDraggingHostIDs,
    loadHosts,
    createHost,
    updateHost,
    deleteHost,
    deleteHosts,
    moveHostsOptimistically,
    reorderHostsOptimistically
  }
}
