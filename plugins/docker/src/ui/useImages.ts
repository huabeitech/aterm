import { useCallback, useEffect, useRef, useState } from 'react'
import type { useDocker } from './useDocker'
export interface DockerImage {
  ID: string
  RepoTags: string[]
  CreatedAt: string
  Size: string
  Containers: number
}
export interface ImageDetail {
  Id: string
  RepoTags: string[]
  RepoDigests: string[]
  Created: string
  Size: number
  Os: string
  Architecture: string
  Layers: number
  Entrypoint: string[]
  Cmd: string[]
}
export function useImages(connection: ReturnType<typeof useDocker>, visible: boolean) {
  const [images, setImages] = useState<DockerImage[]>([]),
    [selected, setSelected] = useState<string | null>(null),
    [detail, setDetail] = useState<ImageDetail | null>(null)
  const [busy, setBusy] = useState(false),
    [ready, setReady] = useState(false),
    [error, setError] = useState(''),
    [detailError, setDetailError] = useState('')
  const version = useRef(0),
    manual = useRef(false),
    polling = useRef(false),
    selection = useRef(selected),
    active = visible && connection.active && !!connection.engine
  selection.current = selected
  const engineKey = connection.engine
    ? `${connection.engine.context}:${connection.engine.engine}`
    : ''
  const previousEngine = useRef(engineKey)
  const { request } = connection
  const refresh = useCallback(async () => {
    if (!active || manual.current) return
    manual.current = true
    setBusy(true)
    setError('')
    const epoch = version.current
    try {
      const data = await request<DockerImage[]>('images.list')
      if (epoch !== version.current) return
      setImages(data)
      setReady(true)
      if (selection.current && !data.some((i) => i.ID === selection.current)) {
        selection.current = null
        setSelected(null)
        setDetail(null)
      }
    } catch (e) {
      if (epoch === version.current) setError((e as Error).message)
    } finally {
      if (epoch === version.current) {
        manual.current = false
        setBusy(false)
      }
    }
  }, [active, request])
  useEffect(() => {
    version.current++
    manual.current = false
    polling.current = false
    setBusy(false)
    if (previousEngine.current !== engineKey) {
      previousEngine.current = engineKey
      setImages([])
      setSelected(null)
      selection.current = null
      setDetail(null)
      setReady(false)
    }
    if (!active) return
    void refresh()
    const timer = setInterval(async () => {
      if (manual.current || polling.current) return
      const epoch = version.current
      polling.current = true
      try {
        const data = await request<DockerImage[]>('images.list')
        if (epoch !== version.current) return
        setImages(data)
        if (selection.current && !data.some((i) => i.ID === selection.current)) {
          selection.current = null
          setSelected(null)
          setDetail(null)
        }
      } catch (e) {
        if (epoch === version.current) setError((e as Error).message)
      } finally {
        if (epoch === version.current) polling.current = false
      }
    }, 5000)
    return () => {
      clearInterval(timer)
      version.current++
      manual.current = false
      polling.current = false
    }
  }, [active, engineKey, refresh, request])
  const choose = useCallback(
    (id: string | null) => {
      selection.current = id
      setSelected(id)
      setDetail(null)
      setDetailError('')
      if (!id || !active) return
      const epoch = version.current
      void request<ImageDetail>('images.inspect', id)
        .then((data) => {
          if (epoch === version.current && selection.current === id) setDetail(data)
        })
        .catch((e) => {
          if (epoch === version.current && selection.current === id)
            setDetailError((e as Error).message)
        })
    },
    [active, request]
  )
  const remove = useCallback(async () => {
    const id = selection.current
    if (!id || !active || manual.current) return
    const epoch = version.current
    manual.current = true
    setBusy(true)
    setError('')
    try {
      await request('images.remove', id)
      if (epoch !== version.current) return
      choose(null)
      const data = await request<DockerImage[]>('images.list')
      if (epoch === version.current) setImages(data)
    } catch (e) {
      if (epoch === version.current) setError((e as Error).message)
    } finally {
      if (epoch === version.current) {
        manual.current = false
        setBusy(false)
      }
    }
  }, [active, choose, request])
  return {
    images,
    selected: images.find((i) => i.ID === selected) ?? null,
    detail,
    busy,
    ready,
    error,
    detailError,
    choose,
    remove,
    refresh,
    active
  }
}
