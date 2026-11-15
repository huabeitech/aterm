import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useId,
  useState,
  type ReactNode
} from 'react'

const HoverDetailsGroupContext = createContext<{
  activeId: string | null
  warm: boolean
  onOpenChange: (id: string, open: boolean) => void
} | null>(null)

// Keep the group warm briefly so crossing gaps between tabs doesn't restart the delay.
export function HoverDetailsGroup({ children }: { children: ReactNode }) {
  const [activeId, setActiveId] = useState<string | null>(null)
  const [warm, setWarm] = useState(false)
  useEffect(() => {
    if (activeId !== null || !warm) return
    const timer = setTimeout(() => setWarm(false), 300)
    return () => clearTimeout(timer)
  }, [activeId, warm])
  const onOpenChange = useCallback((id: string, open: boolean) => {
    if (open) {
      setWarm(true)
      setActiveId(id)
    } else {
      setActiveId((current) => (current === id ? null : current))
    }
  }, [])

  return (
    <HoverDetailsGroupContext.Provider
      value={{
        activeId,
        warm,
        onOpenChange
      }}
    >
      {children}
    </HoverDetailsGroupContext.Provider>
  )
}

export function useHoverDetailsState(disabled = false) {
  const [localOpen, setLocalOpen] = useState(false)
  const group = useContext(HoverDetailsGroupContext)
  const id = useId()
  const groupOpenChange = group?.onOpenChange
  useEffect(() => {
    if (disabled) {
      setLocalOpen(false)
      groupOpenChange?.(id, false)
    }
  }, [disabled, id, groupOpenChange])
  useEffect(() => () => groupOpenChange?.(id, false), [id, groupOpenChange])
  const setOpen = useCallback(
    (next: boolean) => {
      if (groupOpenChange) groupOpenChange(id, !disabled && next)
      else setLocalOpen(!disabled && next)
    },
    [disabled, id, groupOpenChange]
  )
  return {
    open: !disabled && (group ? group.activeId === id : localOpen),
    setOpen,
    warm: group?.warm ?? false,
    grouped: !!group
  }
}
