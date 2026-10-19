import { useState } from 'react'
import { api, V1 } from '../../lib/api'
import { useApp } from '../../lib/app-context'
import type { Fuel } from '../../lib/types'
import { fmtDate } from '../../lib/format'
import { cleanNotes, fmtEcon, fmtNum, fmtOdo, fmtPricePer, fmtVol } from '../../lib/units'
import { Card, Confirm, Empty, Loading, useAsync, useToast } from '../../components/ui'
import { Money } from '../../components/Money'
import { Line } from '../../components/Charts'
import { FuelModal } from '../../components/forms'
import { useVehicle } from './ctx'
import { useMoneyStatus } from '../../components/MoneyBook'

export default function FuelTab() {
  const { vehicle: v, reload: reloadOv, canEdit, overview } = useVehicle()
  const { settings, currencies } = useApp()
  const { toast } = useToast()
  const [q, setQ] = useState('')
  const { data, loading, reload } = useAsync(() => api.get<Fuel[]>(`${V1}/vehicles/${v.id}/fuel`), [v.id])
  const [edit, setEdit] = useState<Fuel | null | 'new'>(null)
  const [del, setDel] = useState<Fuel | null>(null)
  const u = settings!
  const dec = currencies.find(c => c.code === v.currency)?.decimals ?? 2
  const s = v.summary!
  const list = (data || []).filter(f => !q || `${f.station} ${f.notes} ${f.grade} ${f.tags}`.toLowerCase().includes(q.toLowerCase()))
  const done = () => { setEdit(null); reload(); reloadOv() }
  const series = [...(data || [])].reverse().filter(f => f.economy > 0).map(f => ({ x: fmtDate(f.date, false), y: f.economy, label: `${Math.round(f.distance)} ${v.odometer_unit}` }))
  const prices = [...(data || [])].reverse().filter(f => f.volume > 0).map(f => ({ x: fmtDate(f.date, false), y: f.cost_home / f.volume }))
  const ms = useMoneyStatus(v.id)
  const book = async (f: Fuel) => { try { await api.post(`${V1}/vehicles/${v.id}/fuel/${f.id}/money`, {}); toast('Booked in Pocket Money'); reload() } catch (e) { toast((e as Error).message, 'err') } }
  const push = async (f: Fuel) => { try { await api.post(`${V1}/vehicles/${v.id}/fuel/${f.id}/firefly`); toast('Sent to Firefly'); reload() } catch (e) { toast((e as Error).message, 'err') } }
  const ev = v.fuel_type === 'electric'
  return (
    <>
      <div className="stats">
        <div className="stat"><div className="n">{s.fills}</div><div className="l">{ev ? 'charges' : 'fill-ups'} · {fmtVol((data || []).reduce((a, f) => a + f.volume, 0), u, 0)} total</div></div>
        <div className="stat"><div className="n">{s.economy ? fmtEcon(s.economy, u, v.odometer_unit) : '—'}</div><div className="l">average{s.economy_recent ? ` · ${fmtEcon(s.economy_recent, u, v.odometer_unit)} recently` : ''}</div></div>
        <div className="stat"><div className="n"><Money minor={s.cost_fuel_total} code={v.currency} compact /></div><div className="l">spent on {ev ? 'energy' : 'fuel'}</div></div>
        {s.last_fuel && s.last_fuel.volume > 0 && <div className="stat"><div className="n">{fmtPricePer(s.last_fuel.price_per, s.last_fuel.currency, currencies.find(c => c.code === s.last_fuel!.currency)?.decimals ?? 2, u)}</div><div className="l">last price · {fmtDate(s.last_fuel.date, false)}</div></div>}
      </div>
      <div className="two-col mb">
        <Card title={`Economy per tank (${ev ? 'kWh' : 'L'}/100 ${v.odometer_unit})`}><Line points={series} fmt={y => fmtEcon(y, u, v.odometer_unit)} height={150} /></Card>
        <Card title={`Price per ${ev ? 'kWh' : 'litre'}`}><Line points={prices} fmt={y => fmtPricePer(y, v.currency, dec, u)} height={150} /></Card>
      </div>
      <Card title="Fill-ups" actions={<>
        <div className="search" style={{ position: 'relative' }}><span className="ico">⌕</span><input value={q} onChange={e => setQ(e.target.value)} placeholder="Search station, notes…" style={{ paddingLeft: 30, width: 200 }} /></div>
        <a className="btn sm" href={`${V1}/vehicles/${v.id}/export/fuel.csv`}>CSV</a>
        {canEdit && <button className="btn primary sm" onClick={() => setEdit('new')}>+ Fill-up</button>}
      </>} flush>
        {loading && !data ? <Loading /> : list.length === 0 ? <div className="pad"><Empty title={q ? 'No matches' : 'No fill-ups yet'}>{!q && 'Log the first one with the button above, or from your phone at /quick.'}</Empty></div> : (
          <table className="table compact">
            <thead><tr><th>Date</th><th className="num">Odometer</th><th className="num">Distance</th><th className="num">{ev ? 'kWh' : 'Volume'}</th><th className="num">Cost</th><th className="num">Per {ev ? 'kWh' : 'unit'}</th><th className="num">Economy</th><th>Station</th><th /></tr></thead>
            <tbody>{list.map(f => (
              <tr key={f.id} className="fuel-row">
                <td>{fmtDate(f.date)}{!f.full_tank && <span className="badge warn" style={{ marginLeft: 6 }} title="Partial fill">partial</span>}{f.missed && <span className="badge" style={{ marginLeft: 6 }} title="A fill-up before this one was not logged">gap</span>}</td>
                <td className="num">{fmtOdo(f.odometer, v.odometer_unit, false)}</td>
                <td className="num muted">{f.distance > 0 ? fmtNum(f.distance) : '—'}</td>
                <td className="num">{fmtVol(f.volume, u)}</td>
                <td className="num"><Money minor={f.cost} code={f.currency} />{f.currency !== v.currency && <div className="muted small"><Money minor={f.cost_home} code={v.currency} /></div>}</td>
                <td className="num muted small">{f.volume > 0 ? fmtPricePer(f.price_per, f.currency, currencies.find(c => c.code === f.currency)?.decimals ?? 2, u) : ''}</td>
                <td className="num">{f.economy > 0 ? fmtEcon(f.economy, u, v.odometer_unit) : <span className="muted">—</span>}</td>
                <td className="muted">{f.station}{f.grade && <span className="small"> · {f.grade}</span>}{cleanNotes(f.notes) && <div className="small">{cleanNotes(f.notes)}</div>}{f.firefly_id && <span className="tag-src" title={`Firefly journal ${f.firefly_id}`}> · firefly</span>}{f.money_tx_id && <span className="tag-src" title="Booked in Pocket Money"> · money</span>}{f.source !== 'manual' && <span className="tag-src"> · {f.source}</span>}</td>
                <td className="right" style={{ whiteSpace: 'nowrap' }}>{canEdit && <>{ms?.connected && !f.money_tx_id && f.cost > 0 && <button className="btn ghost sm" onClick={() => book(f)} title="Create the transaction in Pocket Money">→ Money</button>}{!ms?.offer && v.role === 'owner' && overview.integrations.firefly && !f.firefly_id && f.cost > 0 && <button className="btn ghost sm" onClick={() => push(f)} title="Create the withdrawal in Firefly III">→ Firefly</button>}<button className="btn ghost sm" onClick={() => setEdit(f)}>Edit</button><button className="btn ghost sm danger-text" onClick={() => setDel(f)}>✕</button></>}</td>
              </tr>
            ))}</tbody>
          </table>
        )}
      </Card>
      {edit && <FuelModal vehicle={v} fuel={edit === 'new' ? undefined : edit} onClose={() => setEdit(null)} onSaved={done} />}
      {del && <Confirm title="Delete this fill-up?" message={`Economy for the neighbouring fill-ups is recomputed.${del.money_tx_id ? ' Its Pocket Money transaction is deleted too.' : ''}`} onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/vehicles/${v.id}/fuel/${del.id}`); setDel(null); reload(); reloadOv() }} />}
    </>
  )
}
