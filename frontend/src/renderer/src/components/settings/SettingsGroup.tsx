import { useId, type ReactNode } from 'react'

export function SettingsGroup({
  title,
  footer,
  children
}: {
  title: string
  footer?: string
  children: ReactNode
}) {
  const id = useId()
  return (
    <section className="settings-group" aria-labelledby={id}>
      <h2 id={id}>{title}</h2>
      <div className="settings-group-rows">{children}</div>
      {footer && <p className="settings-footnote">{footer}</p>}
    </section>
  )
}

export function SettingRow({
  label,
  children,
  error
}: {
  label: string
  children: ReactNode
  error?: string
}) {
  return (
    <div className="settings-row">
      <span className="settings-row-label">{label}</span>
      <div className="settings-row-control">{children}</div>
      {error && (
        <p className="settings-inline-error" role="alert">
          {error}
        </p>
      )}
    </div>
  )
}
