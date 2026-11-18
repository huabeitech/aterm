import { useHoverDetailsState } from '@/components/ui/hover-details-state'
import {
  forwardRef,
  type ReactElement,
  type ComponentPropsWithoutRef,
  type ComponentRef
} from 'react'
import { HoverCard } from 'radix-ui'
import { Server } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { t } from '../../../../shared/i18n'
import type { domain } from '../../api/types'

// Informational only: keep connecting, selection and drag/drop on the tree row.
type Props = Omit<ComponentPropsWithoutRef<typeof HoverCard.Trigger>, 'children'> & {
  host: domain.Host
  directoryName: string
  disabled: boolean
  children: ReactElement
}

// Forward trigger props/ref so Radix's context-menu trigger can compose with this card.
export const ConnectionHoverCard = forwardRef<ComponentRef<typeof HoverCard.Trigger>, Props>(
  function ConnectionHoverCard({ host, directoryName, disabled, children, ...triggerProps }, ref) {
    useTranslation()
    const { open, setOpen, warm } = useHoverDetailsState(disabled)
    const details = [
      [t('hosts:address'), host.hostName],
      [t('hosts:port'), String(host.port || 22)],
      [t('hosts:username'), host.user || '—'],
      [t('hosts:directory'), directoryName],
      [t('hosts:authentication'), host.passwordAuth ? t('hosts:password') : t('hosts:publicKey')],
      ...(host.proxyJump ? [[t('hosts:jumpHost'), host.proxyJump]] : [])
    ]
    return (
      <HoverCard.Root
        open={open}
        onOpenChange={setOpen}
        openDelay={warm ? 250 : 350}
        closeDelay={300}
      >
        <HoverCard.Trigger
          {...triggerProps}
          ref={ref}
          asChild
          onFocus={(event) => {
            triggerProps.onFocus?.(event)
            if (event.target.matches(':focus-visible')) setOpen(true)
          }}
          onBlur={(event) => {
            setOpen(false)
            triggerProps.onBlur?.(event)
          }}
          onPointerDown={(event) => {
            setOpen(false)
            triggerProps.onPointerDown?.(event)
          }}
          onContextMenu={(event) => {
            setOpen(false)
            triggerProps.onContextMenu?.(event)
          }}
          onDragStart={(event) => {
            setOpen(false)
            triggerProps.onDragStart?.(event)
          }}
          onKeyDown={(event) => {
            triggerProps.onKeyDown?.(event)
            if (event.key === 'Escape' || event.key === 'Enter') setOpen(false)
          }}
        >
          {children}
        </HoverCard.Trigger>
        <HoverCard.Portal>
          <HoverCard.Content
            className="connection-hover-card z-50 w-72 max-w-[calc(100vw-24px)] rounded-lg border border-border bg-popover p-3 text-popover-foreground shadow-lg outline-none"
            side="right"
            align="start"
            sideOffset={6}
            arrowPadding={8}
            collisionPadding={12}
            aria-label={`${host.name} · ${t('hosts:details')}`}
            onEscapeKeyDown={() => setOpen(false)}
          >
            <div className="mb-3 flex items-start gap-2 border-b border-border pb-2">
              <Server size={16} className="mt-0.5 shrink-0 text-muted-foreground" />
              <span className="min-w-0 wrap-anywhere text-sm font-semibold">{host.name}</span>
            </div>
            <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1.5 text-xs">
              {details.map(([label, value]) => (
                <div key={label} className="contents">
                  <dt className="text-muted-foreground">{label}</dt>
                  <dd className="m-0 min-w-0 select-text wrap-anywhere">{value}</dd>
                </div>
              ))}
            </dl>
            {host.remark && (
              <p className="mb-0 mt-3 max-h-32 overflow-auto whitespace-pre-wrap wrap-anywhere border-t border-border pt-2 text-xs text-muted-foreground">
                {host.remark}
              </p>
            )}
            <HoverCard.Arrow asChild width={12} height={6}>
              <svg
                className="connection-hover-arrow overflow-visible fill-popover"
                viewBox="0 0 12 6"
                aria-hidden="true"
              >
                {/* Overlap the panel border so the arrow and surface join seamlessly. */}
                <path d="M0 -1H12L6 6Z" />
                <path d="M0 0L6 6L12 0" fill="none" className="stroke-border" />
              </svg>
            </HoverCard.Arrow>
          </HoverCard.Content>
        </HoverCard.Portal>
      </HoverCard.Root>
    )
  }
)
