import { useMemo, useState } from 'react'
import { useApp } from '../lib/app-context'
import { fmtMoney } from '../lib/format'
import { KIND_LABEL, SERIES, SERIES_ORDER, type MonthlyCost } from '../lib/types'

const MON = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
function monthLabel(m: string): string { const [y, mm] = m.split('-'); return `${MON[Number(mm) - 1]}${mm === '01' ? ` ${y.slice(2)}` : ''}` }
function useSeriesColor() { const { theme } = useApp(); return (k: string) => (SERIES[k] || SERIES.expense)[theme] }

/** Stacked monthly cost bars: one column per month, a 2px surface gap between segments, legend below, hover tooltip. */
export function CostBars({ data, currency, decimals = 2, height = 180 }: { data: MonthlyCost[]; currency: string; decimals?: number; height?: number }) {
  const color = useSeriesColor()
  const [hover, setHover] = useState<number | null>(null)
  const kinds = useMemo(() => SERIES_ORDER.filter(k => data.some(d => (d.by_kind[k] || 0) > 0)), [data])
  const max = Math.max(1, ...data.map(d => d.total))
  const W = 100, H = height, padB = 2, padT = 6
  const n = Math.max(1, data.length)
  const slot = W / n
  const bw = Math.min(slot * 0.68, 9)
  const y = (v: number) => padT + (H - padT - padB) * (1 - v / max)
  if (data.length === 0) return <p className="muted small">No costs in this period yet.</p>
  const h = hover == null ? null : data[hover]
  return (
    <div className="chart">
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="chart-svg" style={{ height }} role="img" aria-label="Costs per month">
        {[0.5, 1].map(f => <line key={f} x1="0" x2={W} y1={y(max * f)} y2={y(max * f)} className="grid-line" />)}
        {data.map((d, i) => {
          let acc = 0
          const x = i * slot + (slot - bw) / 2
          return (
            <g key={d.month} onMouseEnter={() => setHover(i)} onMouseLeave={() => setHover(null)} onTouchStart={() => setHover(i)}>
              <rect x={i * slot} y={0} width={slot} height={H} fill="transparent" />
              {kinds.map(k => {
                const v = d.by_kind[k] || 0
                if (!v) return null
                const y0 = y(acc + v), y1 = y(acc)
                acc += v
                return <rect key={k} x={x} y={y0} width={bw} height={Math.max(0, y1 - y0 - 0.6)} fill={color(k)} opacity={hover == null || hover === i ? 1 : 0.45} rx={acc === d.total ? 1 : 0} />
              })}
            </g>
          )
        })}
      </svg>
      <div className="chart-axis" style={{ gridTemplateColumns: `repeat(${n}, 1fr)` }}>{data.map((d, i) => <span key={d.month}>{(n <= 12 || i % Math.ceil(n / 12) === 0) ? monthLabel(d.month) : ''}</span>)}</div>
      {h && <div className="chart-tip"><b>{monthLabel(h.month)}</b> · {fmtMoney(h.total, currency, decimals)}{kinds.map(k => h.by_kind[k] ? <span key={k}><span className="dot" style={{ background: color(k) }} />{KIND_LABEL[k]} {fmtMoney(h.by_kind[k], currency, decimals)}</span> : null)}</div>}
      {kinds.length > 1 && <div className="legend">{kinds.map(k => <span key={k}><span className="dot" style={{ background: color(k) }} />{KIND_LABEL[k]}</span>)}</div>}
    </div>
  )
}

export interface Pt { x: string; y: number; label?: string }

/** One-series line with hover crosshair. `fmt` renders the value for the tooltip. */
export function Line({ points, fmt, height = 160, color, baseline }: { points: Pt[]; fmt: (y: number) => string; height?: number; color?: string; baseline?: number }) {
  const { theme } = useApp()
  const c = color || SERIES.fuel[theme]
  const [hover, setHover] = useState<number | null>(null)
  const pts = points.filter(p => Number.isFinite(p.y))
  if (pts.length < 2) return <p className="muted small">Not enough data yet — two full tanks make the first point.</p>
  const W = 100, H = height, padT = 8, padB = 4, padX = 2
  const ys = pts.map(p => p.y)
  let lo = Math.min(...ys), hi = Math.max(...ys)
  if (baseline != null) { lo = Math.min(lo, baseline); hi = Math.max(hi, baseline) }
  if (hi === lo) { hi += 1; lo -= 1 }
  const pad = (hi - lo) * 0.1
  lo -= pad; hi += pad
  const x = (i: number) => padX + (W - 2 * padX) * (pts.length === 1 ? 0.5 : i / (pts.length - 1))
  const y = (v: number) => padT + (H - padT - padB) * (1 - (v - lo) / (hi - lo))
  const d = pts.map((p, i) => `${i ? 'L' : 'M'}${x(i).toFixed(2)},${y(p.y).toFixed(2)}`).join(' ')
  const avg = ys.reduce((a, b) => a + b, 0) / ys.length
  const h = hover == null ? null : pts[hover]
  const onMove = (e: React.MouseEvent<SVGSVGElement> | React.TouchEvent<SVGSVGElement>) => {
    const r = (e.currentTarget as SVGSVGElement).getBoundingClientRect()
    const cx = 'touches' in e ? e.touches[0].clientX : e.clientX
    const fx = ((cx - r.left) / r.width) * W
    let best = 0
    for (let i = 1; i < pts.length; i++) if (Math.abs(x(i) - fx) < Math.abs(x(best) - fx)) best = i
    setHover(best)
  }
  return (
    <div className="chart">
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="chart-svg" style={{ height }} onMouseMove={onMove} onTouchMove={onMove} onMouseLeave={() => setHover(null)} role="img">
        <line x1="0" x2={W} y1={y(avg)} y2={y(avg)} className="grid-line dashed" />
        {baseline != null && <line x1="0" x2={W} y1={y(baseline)} y2={y(baseline)} className="grid-line" />}
        <path d={d} fill="none" stroke={c} strokeWidth="1.6" vectorEffect="non-scaling-stroke" strokeLinejoin="round" strokeLinecap="round" />
        {hover != null && <>
          <line x1={x(hover)} x2={x(hover)} y1={padT} y2={H - padB} className="grid-line" />
          <circle cx={x(hover)} cy={y(pts[hover].y)} r="2.2" fill={c} className="pt-ring" vectorEffect="non-scaling-stroke" />
        </>}
      </svg>
      <div className="chart-axis two"><span>{pts[0].x}</span><span>{pts[pts.length - 1].x}</span></div>
      <div className="chart-tip">{h ? <><b>{h.x}</b> · {fmt(h.y)}{h.label && <span className="muted"> {h.label}</span>}</> : <span className="muted">average {fmt(avg)} · hover for values</span>}</div>
    </div>
  )
}

/** Tiny inline trend, no axes. */
export function Spark({ values, color, width = 90, height = 26 }: { values: number[]; color?: string; width?: number; height?: number }) {
  const { theme } = useApp()
  const c = color || SERIES.fuel[theme]
  const v = values.filter(Number.isFinite)
  if (v.length < 2) return null
  const lo = Math.min(...v), hi = Math.max(...v) || 1
  const d = v.map((p, i) => `${i ? 'L' : 'M'}${(i / (v.length - 1) * width).toFixed(1)},${(height - 2 - (height - 4) * ((p - lo) / (hi - lo || 1))).toFixed(1)}`).join(' ')
  return <svg width={width} height={height} className="spark" aria-hidden><path d={d} fill="none" stroke={c} strokeWidth="1.5" strokeLinejoin="round" /></svg>
}

/** Horizontal bars for category totals (sequential: one hue, longest first). */
export function HBars({ rows, currency, decimals = 2, max: maxRows = 10 }: { rows: { label: string; value: number }[]; currency: string; decimals?: number; max?: number }) {
  const { theme } = useApp()
  const sorted = [...rows].filter(r => r.value > 0).sort((a, b) => b.value - a.value).slice(0, maxRows)
  const max = Math.max(1, ...sorted.map(r => r.value))
  if (sorted.length === 0) return <p className="muted small">Nothing yet.</p>
  return (
    <div className="hbars">
      {sorted.map(r => <div key={r.label} className="hbar"><span className="lbl">{r.label}</span><span className="track"><span className="fill" style={{ width: `${r.value / max * 100}%`, background: SERIES.fuel[theme] }} /></span><span className="val">{fmtMoney(r.value, currency, decimals, { compact: true })}</span></div>)}
    </div>
  )
}
