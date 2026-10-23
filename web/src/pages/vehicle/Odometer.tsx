import { useState } from 'react'
import { api, V1 } from '../../lib/api'
import type { OdometerReading } from '../../lib/types'
import { fmtDate } from '../../lib/format'
import { fmtNum, fmtOdo } from '../../lib/units'
import { Card, Confirm, Empty, Loading, useAsync, useToast } from '../../components/ui'
import { Line } from '../../components/Charts'
import { OdometerModal } from '../../components/forms'
import { useVehicle } from './ctx'
import { Icon } from '../../components/icons'

interface Hist { history: OdometerReading[]; latest: number; latest_date: string; estimate_km: number; estimate: number }

export default function OdometerTab() {
  const { vehicle: v, reload: reloadOv, canEdit, overview } = useVehicle()
  const { toast } = useToast()
  const { data, loading, reload } = useAsync(() => api.get<Hist>(`${V1}/vehicles/${v.id}/odometer`), [v.id])
  const [edit, setEdit] = useState<OdometerReading | null | 'new'>(null)
  const [del, setDel] = useState<OdometerReading | null>(null)
  const [busy, setBusy] = useState(false)
  const confirm = async () => { setBusy(true); try { await api.post(`${V1}/vehicles/${v.id}/odometer/confirm`); toast('Estimate confirmed as a reading'); reload(); reloadOv() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) } }
  const sync = async () => { setBusy(true); try { const r = await api.post<{ days: number; source_name?: string }>(`${V1}/vehicles/${v.id}/dawarich/sync`); toast(`${r.source_name || 'Dawarich'}: ${r.days} days refreshed`); reload(); reloadOv() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) } }
  if (loading && !data) return <Loading />
  if (!data) return null
  data.history = data.history || []
  const series = [...data.history].reverse().map(o => ({ x: fmtDate(o.date, false), y: o.reading }))
  const locName = overview.location?.name || 'Dawarich'
  const src: Record<string, string> = { manual: 'reading', api: 'app', dawarich: 'Dawarich', lubelogger: 'LubeLogger', fuel: 'fill-up', record: 'service' }
  return (
    <>
      <div className="stats">
        <div className="stat"><div className="odo-big">{fmtOdo(data.latest, v.odometer_unit, false)}<small>{v.odometer_unit}</small></div><div className="l">last confirmed{data.latest_date && ` · ${fmtDate(data.latest_date)}`}</div></div>
        {data.estimate_km > 0 && <div className="stat"><div className="odo-big">{fmtOdo(data.estimate, v.odometer_unit, false)}<small>{v.odometer_unit}</small></div><div className="l">estimate · +{fmtNum(data.estimate_km)} {v.odometer_unit} driven since, per {locName} {canEdit && <button className="btn sm" onClick={confirm} disabled={busy} style={{ marginLeft: 6 }}>Confirm as reading</button>}</div></div>}
        {v.summary && v.summary.km_per_year > 0 && <div className="stat"><div className="n">{fmtNum(v.summary.km_per_year)}</div><div className="l">{v.odometer_unit} per year</div></div>}
      </div>
      {overview.integrations.dawarich ? <p className="muted small mb">{locName}'s driving kilometres per day are added on top of your last reading. Confirm the estimate whenever you glance at the dash — that resets the error. <button className="btn ghost sm" onClick={sync} disabled={busy}>Refresh from {locName}</button></p>
        : v.dawarich ? <p className="muted small mb">Location history is switched on for this vehicle but neither Pocket Movement nor Dawarich is connected under Settings → Connected apps.</p> : <p className="muted small mb">Turn on location history under this vehicle's Settings to estimate the odometer between readings from your recorded drives.</p>}
      <div className="two-col mb">
        <Card title="Odometer over time"><Line points={series} fmt={y => fmtOdo(y, v.odometer_unit)} height={160} /></Card>
        <Card title="Readings" actions={<>{canEdit && <button className="btn primary sm" onClick={() => setEdit('new')}>+ Reading</button>}<a className="btn sm" href={`${V1}/vehicles/${v.id}/export/odometer.csv`}>CSV</a></>} flush>
          {data.history.length === 0 ? <div className="pad"><Empty title="No readings yet">Fill-ups and service records with an odometer count as readings — no double entry.</Empty></div> : (
            <table className="table compact">
              <thead><tr><th>Date</th><th className="num">Reading</th><th className="num">Δ</th><th>Source</th><th /></tr></thead>
              <tbody>{data.history.map(o => <tr key={`${o.source}-${o.id}`}>
                <td>{fmtDate(o.date)}</td><td className="num">{fmtOdo(o.reading, v.odometer_unit, false)}</td>
                <td className="num muted small">{o.delta ? `${o.delta > 0 ? '+' : ''}${fmtNum(o.delta)}${o.days ? ` / ${o.days} d` : ''}` : ''}</td>
                <td className="muted small">{src[o.source] || o.source}{o.notes && <> · {o.notes}</>}</td>
                <td className="right">{canEdit && (o.source === 'manual' || o.source === 'api' || o.source === 'dawarich' || o.source === 'lubelogger') && <><button className="btn ghost sm" onClick={() => setEdit(o)}>Edit</button><button className="btn ghost sm icon-only danger-text" onClick={() => setDel(o)} aria-label="Delete" title="Delete"><Icon name="trash" size={14} /></button></>}</td>
              </tr>)}</tbody>
            </table>
          )}
        </Card>
      </div>
      {edit && <OdometerModal vehicle={v} reading={edit === 'new' ? undefined : edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reload(); reloadOv() }} />}
      {del && <Confirm title="Delete this reading?" message="Only the standalone reading is removed." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/vehicles/${v.id}/odometer/${del.id}`); setDel(null); reload(); reloadOv() }} />}
    </>
  )
}
