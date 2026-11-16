import { useEffect, useRef, useState } from 'react'
import { trendCeiling, trendSegments } from './resourceModel'

/** Responsive SVG coordinates use CSS pixels, so sparse samples never stretch into ovals. */
export function ResourceTrend({
  points,
  interval,
  ceiling,
  label,
  windowLabel,
  nowLabel,
  emptyLabel,
  formatValue
}: {
  points: { at: number; value: number | null }[]
  interval: number
  ceiling?: number
  label: string
  windowLabel: string
  nowLabel: string
  emptyLabel: string
  formatValue: (value: number | null) => string
}) {
  const plot = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(280)
  const [selected, setSelected] = useState<number>()
  useEffect(() => {
    const element = plot.current
    if (!element) return
    const observer = new ResizeObserver(([entry]) => setWidth(Math.max(8, entry.contentRect.width)))
    observer.observe(element)
    return () => observer.disconnect()
  }, [])
  const max = ceiling ?? trendCeiling(points)
  const segments = trendSegments(points, interval, max, width)
  const samples = segments.flat()
  const last = samples.at(-1)
  const hovered =
    selected === undefined
      ? undefined
      : samples.reduce<(typeof samples)[number] | undefined>(
          (nearest, point) =>
            !nearest || Math.abs(point.at - selected) < Math.abs(nearest.at - selected)
              ? point
              : nearest,
          undefined
        )
  const path = (segment: typeof samples) =>
    segment.map((p, i) => `${i ? 'L' : 'M'}${p.x},${p.y}`).join(' ')
  function inspect(x: number) {
    const nearest = samples.reduce<(typeof samples)[number] | undefined>(
      (before, point) =>
        !before || Math.abs(point.x - x) < Math.abs(before.x - x) ? point : before,
      undefined
    )
    setSelected(nearest?.at)
  }
  return (
    <figure className="resource-trend" aria-label={label}>
      <div className="resource-trend-axis" aria-hidden="true">
        <span>{formatValue(max)}</span>
        <span>{formatValue(max / 2)}</span>
        <span>{formatValue(0)}</span>
      </div>
      <div
        className="resource-trend-plot"
        ref={plot}
        tabIndex={samples.length ? 0 : undefined}
        aria-label={label}
        onPointerMove={(event) =>
          inspect(event.clientX - event.currentTarget.getBoundingClientRect().left)
        }
        onPointerLeave={() => setSelected(undefined)}
        onBlur={() => setSelected(undefined)}
        onFocus={() => setSelected(last?.at)}
        onKeyDown={(event) => {
          if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
          event.preventDefault()
          const current = samples.findIndex((p) => p.at === hovered?.at)
          const index =
            event.key === 'Home'
              ? 0
              : event.key === 'End'
                ? samples.length - 1
                : Math.max(
                    0,
                    Math.min(samples.length - 1, current + (event.key === 'ArrowLeft' ? -1 : 1))
                  )
          setSelected(samples[index]?.at)
        }}
      >
        <svg viewBox={`0 0 ${width} 96`} role="img" aria-label={label}>
          {[4, 48, 92].map((y) => (
            <line key={y} className="resource-trend-grid" x1={0} x2={width} y1={y} y2={y} />
          ))}
          {segments.map((segment, index) => (
            <g key={index}>
              {segment.length > 1 && (
                <>
                  <path
                    className="resource-trend-area"
                    d={`${path(segment)} L${segment.at(-1)!.x},92 L${segment[0].x},92 Z`}
                  />
                  <path className="resource-trend-line" d={path(segment)} />
                </>
              )}
              {segment.length === 1 && (
                <circle
                  className="resource-trend-point"
                  cx={segment[0].x}
                  cy={segment[0].y}
                  r={2}
                />
              )}
            </g>
          ))}
          {last && <circle className="resource-trend-end" cx={last.x} cy={last.y} r={3} />}
          {hovered && (
            <g>
              <line
                className="resource-trend-cursor"
                x1={hovered.x}
                x2={hovered.x}
                y1={4}
                y2={92}
              />
              <circle className="resource-trend-end" cx={hovered.x} cy={hovered.y} r={3} />
            </g>
          )}
        </svg>
        {!samples.length && <span className="resource-trend-empty">{emptyLabel}</span>}
        {hovered && (
          <div
            className="resource-trend-tooltip"
            role="tooltip"
            style={{ left: Math.max(0, Math.min(width - 104, hovered.x - 52)) }}
          >
            <strong>{formatValue(hovered.value)}</strong>
            <span>{new Date(hovered.at).toLocaleTimeString(undefined, { hour12: false })}</span>
          </div>
        )}
      </div>
      <figcaption>
        <span>{windowLabel}</span>
        <span>{nowLabel}</span>
      </figcaption>
    </figure>
  )
}
