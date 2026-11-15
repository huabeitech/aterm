import type { ReactNode } from 'react'
import { FolderOpen } from 'lucide-react'

export function FileState({
  title,
  description,
  icon,
  children
}: {
  title: string
  description: string
  icon?: ReactNode
  children?: ReactNode
}) {
  return (
    <div className="files-empty" role="status">
      <span className="files-empty-icon" aria-hidden>
        {icon ?? <FolderOpen size={24} />}
      </span>
      <h3>{title}</h3>
      <p>{description}</p>
      {children && <div className="files-empty-actions">{children}</div>}
    </div>
  )
}

export function FileListLoading({ title }: { title: string }) {
  return (
    <div className="files-loading" role="status" aria-label={title}>
      <span className="sr-only">{title}</span>
      <div className="files-skeleton-heading" aria-hidden />
      {Array.from({ length: 6 }, (_, index) => (
        <div className="files-skeleton-row" key={index} aria-hidden>
          <span className="files-skeleton-icon" />
          <span className="files-skeleton-name" style={{ width: `${32 + (index % 3) * 16}%` }} />
          <span className="files-skeleton-meta" />
        </div>
      ))}
    </div>
  )
}
