import { useState } from 'react'
import { api, V1 } from '../../lib/api'
import { useApp } from '../../lib/app-context'
import type { MonthlyCost, YearlyCost } from '../../lib/types'
import { KIND_LABEL, SERIES_ORDER } from '../../lib/types'
import { fmtDate } from '../../lib/format'
import { fmtEcon, fmtNum, fmtPricePer, fmtVol } from '../../lib/units'
import { Card, Loading, Segmented, useAsync } from '../../components/ui'
import { Money } from '../../components/Money'
import { CostBars, HBars, Line } from '../../components/Charts'
import { useVehicle } from './ctx'

interface Report {
  months: MonthlyCost[]; years: YearlyCost[]; stations: { station: string; fills: number; volume: number; cost: number; price_per: number }[]
  fuel: { date: string; odometer: number; economy: number; price_per: number; volume: number; cost_home: number; distance: number }[]
  odometer: { date: string; reading: number }[]; categories: Record<string, number>; dawarich_days: { date: string; km: number }[]; currency: string
}

export default function Reports() {
  const { vehicle: v } = useVehicle()
  const { settings, currencies } = useApp()
  const [months, setMonths] = useState(24)
  const { data, loading } = useAsync(() => api.get<Report>(`${V1}/vehicles/${v.id}/reports?months=${months}`), [v.id, months])
  const u = settings!
  const dec = currencies.find(c => c.code === v.currency)?.decimals ?? 2
  if (loading && !data) return <Loading />
  if (!data) return null
  const kinds = SERIES_ORDER.filter(k => data.years.some(y => (y.by_kind[k] || 0) > 0))
  const econ = data.fuel.filter(f => f.economy > 0).map(f => ({ x: fmtDate(f.date, false), y: f.economy, label: `${fmtNum(f.distance)} ${v.odometer_unit}` }))
  const price = data.fuel.filter(f => f.volume > 0).map(f => ({ x: fmtDate(f.date, false), y: f.price_per }))
  const totalDays = data.dawarich_days.reduce((a, d) => a + d.km, 0)
  return (
    <>
      <Card title="Costs per month" actions={<Segmented value={months} onChange={setMonths} options={[{ value: 12, label: '12 m' }, { value: 24, label: '24 m' }, { value: 60, label: '5 y' }]} size="sm" />} className="mb">
        <CostBars data={data.months} currency={v.currency} decimals={dec} height={200} />
      </Card>
      <Card title="Per year" className="mb" flush>
        <table className="table compact">
          <thead><tr><th>Year</th>{kinds.map(k => <th key={k} className="num">{KIND_LABEL[k]}</th>)}<th className="num">Total</th><th className="num">{v.odometer_unit}</th><th className="num">per {v.odometer_unit}</th><th className="num">Fills</th><th className="num">Volume</th></tr></thead>
          <tbody>{[...data.years].reverse().map(y => <tr key={y.year}><td><b>{y.year}</b></td>{kinds.map(k => <td key={k} className="num">{y.by_kind[k] ? <Money minor={y.by_kind[k]} code={v.currency} compact /> : <span className="muted">—</span>}</td>)}<td className="num"><b><Money minor={y.total} code={v.currency} compact /></b></td><td className="num">{y.km > 0 ? fmtNum(y.km) : '—'}</td><td className="num muted">{y.km > 0 ? (y.total / y.km / Math.pow(10, dec)).toFixed(2) : '—'}</td><td className="num">{y.fills || '—'}</td><td className="num">{y.volume > 0 ? fmtVol(y.volume, u, 0) : '—'}</td></tr>)}</tbody>
        </table>
      </Card>
      <div className="two-col mb">
        <Card title="Where the money went"><HBars rows={Object.entries(data.categories).map(([label, value]) => ({ label, value }))} currency={v.currency} decimals={dec} /></Card>
        <Card title="Stations"><table className="table compact"><thead><tr><th>Station</th><th className="num">Fills</th><th className="num">Volume</th><th className="num">Avg price</th><th className="num">Spent</th></tr></thead><tbody>{data.stations.map(s => <tr key={s.station}><td>{s.station}</td><td className="num">{s.fills}</td><td className="num">{fmtVol(s.volume, u, 0)}</td><td className="num muted">{s.volume > 0 ? fmtPricePer(s.price_per, v.currency, dec, u) : ''}</td><td className="num"><Money minor={s.cost} code={v.currency} compact /></td></tr>)}</tbody></table></Card>
      </div>
      <div className="two-col mb">
        <Card title="Economy per tank"><Line points={econ} fmt={y => fmtEcon(y, u, v.odometer_unit)} height={170} /></Card>
        <Card title="Price per unit"><Line points={price} fmt={y => fmtPricePer(y, v.currency, dec, u)} height={170} /></Card>
      </div>
      <div className="two-col">
        <Card title="Odometer"><Line points={data.odometer.map(o => ({ x: fmtDate(o.date, false), y: o.reading }))} fmt={y => `${fmtNum(y)} ${v.odometer_unit}`} height={170} /></Card>
        <Card title="Driven per day (Dawarich, last year)">{data.dawarich_days.length === 0 ? <p className="muted small">No recorded days yet. Enable Dawarich for this vehicle under Settings.</p> : <><Line points={data.dawarich_days.map(d => ({ x: fmtDate(d.date, false), y: d.km }))} fmt={y => `${fmtNum(y, 1)} ${v.odometer_unit}`} height={170} /><p className="muted small">{fmtNum(totalDays)} {v.odometer_unit} recorded over {data.dawarich_days.length} days · {fmtNum(totalDays / Math.max(1, data.dawarich_days.length), 1)} a day</p></>}</Card>
      </div>
    </>
  )
}
