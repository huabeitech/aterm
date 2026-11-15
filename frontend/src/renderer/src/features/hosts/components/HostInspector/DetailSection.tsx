import { cn } from '@/lib/utils'
import type { ReactNode } from 'react'

export function DetailSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="detail-section min-w-0">
      <h3
        className={cn(
          'section-label [:root[data-theme=dark]_&]:text-[var(--mac-secondary-text)] mt-0 mr-0 mb-1.5 ml-0 py-0 px-[9px] text-[rgba(60,_60,_67,_0.52)]',
          'text-xs font-bold leading-[calc(1rem*var(--app-font-scale))] [letter-spacing:0] uppercase [.collection-heading_&]:p-0 [.detail-section_&]:p-0 [.detail-section_&]:text-[length:var(--app-text-13)]',
          '[.detail-section_&]:leading-[calc(1.125rem*var(--app-font-scale))] [.host-editor_.detail-section_>_&]:mb-2.5'
        )}
      >
        {title}
      </h3>
      {children}
    </section>
  )
}
