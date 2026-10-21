import { Link } from 'react-router-dom'
import { useApp } from '../../lib/app-context'
import { fmtDate, fmtDateTime, plural } from '../../lib/format'
import { fmtEcon, fmtNum, fmtOdo, fmtPricePer, fmtVol } from '../../lib/units'
import { KIND_LABEL } from '../../lib/types'
import { Card, Empty } from '../../components/ui'
import { Money } from '../../components/Money'
import { CostBars, Spark } from '../../components/Charts'
import { ReminderRow } from '../Reminders'
import { useVehicle } from './ctx'
import { Icon, LOG_KIND_ICON } from '../../components/icons'

export default function OverviewTab() {
  const { vehicle: v, overview: ov, reload } = useVehicle()
  const { settings, currencies } = useApp()
  const s = v.summary!
  const u = settings!
  const dec = currencies.find(c => c.code === v.currency)?.decimals ?? 2
  const attention = ov.reminders.filter(r => r.urgency !== 'ok')
  const econSeries = [...ov.recent_fuel].reverse().map(f => f.economy).filter(x => x > 0)
  return (
    <>
      <div className="stats">
        <div className="stat"><div className="odo-big">{fmtOdo(s.odometer_estimate, v.odometer_unit, false)}<small>{v.odometer_unit}</small></div><div className="l">{s.estimate_km > 0 ? <>estimate · last read {fmtOdo(s.odometer, v.odometer_unit)} on {fmtDate(s.odometer_date, false)} <Link to={`/vehicles/${v.id}/odometer`}>confirm</Link></> : s.odometer_date ? `read on ${fmtDate(s.odometer_date)}` : 'no reading yet'}</div></div>
        <div className="stat"><div className="n">{s.economy ? fmtEcon(s.economy_recent || s.economy, u, v.odometer_unit) : '—'}</div><div className="l">{s.economy_recent ? `last 5 tanks · ${fmtEcon(s.economy, u, v.odometer_unit)} all time` : 'fuel economy'} {econSeries.length > 2 && <Spark values={econSeries} />}</div></div>
        <div className="stat"><div className="n"><Money minor={s.cost_year} code={v.currency} compact /></div><div className="l">this year · <Money minor={s.cost_month} code={v.currency} compact /> this month</div></div>
        <div className="stat"><div className="n">{s.cost_per_km ? `${(s.cost_per_km / Math.pow(10, dec)).toFixed(2)}` : '—'}<span className="muted" style={{ fontSize: 13 }}> {v.currency}/{v.odometer_unit}</span></div><div className="l">all-in cost per {v.odometer_unit}{s.fuel_per_km ? ` · fuel ${(s.fuel_per_km / Math.pow(10, dec)).toFixed(2)}` : ''}</div></div>
        <div className="stat"><div className="n"><Money minor={s.cost_total} code={v.currency} compact /></div><div className="l">total since {v.purchase_date ? fmtDate(v.purchase_date, false) : 'the start'}{s.km_per_year > 0 && <> · ~{fmtNum(s.km_per_year)} {v.odometer_unit}/yr</>}</div></div>
      </div>

      <div className="overview-grid mb">
        <Card title="Needs attention" actions={<Link className="btn ghost sm" to={`/vehicles/${v.id}/reminders`}>All {plural(s.reminders, 'reminder')}</Link>}>
          {attention.length === 0 ? <p className="muted">Nothing due{s.next_reminder ? <>. Next: <b>{s.next_reminder.title}</b>{s.next_reminder.days_left != null && ` in ${s.next_reminder.days_left} days`}{s.next_reminder.km_left != null && ` / ${Math.round(s.next_reminder.km_left).toLocaleString()} ${v.odometer_unit}`}</> : s.reminders === 0 ? <>. <Link to={`/vehicles/${v.id}/reminders`}>Add reminders</Link> for oil, registration, tyres…</> : '.'}</p> : attention.slice(0, 5).map(r => <ReminderRow key={r.id} r={r} vehicle={v} onChanged={reload} />)}
          {ov.expiring_documents.length > 0 && <div className="mt-sm small">{ov.expiring_documents.map(d => <div key={d.id} className="row" style={{ gap: 6 }}><Icon name="file" size={14} className="muted" /><b>{d.name}</b> {d.expires_on < new Date().toISOString().slice(0, 10) ? <span className="bad">expired {fmtDate(d.expires_on)}</span> : <>expires {fmtDate(d.expires_on)}</>}</div>)}</div>}
        </Card>
        <Card title="Costs, last 12 months" actions={<Link className="btn ghost sm" to={`/vehicles/${v.id}/reports`}>Reports</Link>}>
          <CostBars data={ov.months} currency={v.currency} decimals={dec} height={150} />
        </Card>
      </div>

      <div className="overview-grid mb">
        <Card title="Recent fill-ups" actions={<Link className="btn ghost sm" to={`/vehicles/${v.id}/fuel`}>All {plural(s.fills, 'fill-up')}</Link>}>
          {ov.recent_fuel.length === 0 ? <Empty title="No fill-ups yet" /> : <div className="inline-list">{ov.recent_fuel.slice(0, 6).map(f => <div key={f.id} className="li"><Icon name="fuel" size={15} /><span className="grow"><b>{fmtVol(f.volume, u)}</b>{f.station && <span className="muted"> · {f.station}</span>}<span className="muted small"> · {fmtOdo(f.odometer, v.odometer_unit)}</span></span><span className="muted small">{f.economy > 0 ? fmtEcon(f.economy, u, v.odometer_unit) : f.distance > 0 ? `${Math.round(f.distance)} ${v.odometer_unit}` : ''}</span><span className="muted small">{fmtDate(f.date, false)}</span><Money minor={f.cost} code={f.currency} /></div>)}</div>}
          {s.last_fuel && s.last_fuel.volume > 0 && <p className="muted small mt-sm">Last price {fmtPricePer(s.last_fuel.price_per, s.last_fuel.currency, dec, u)}</p>}
        </Card>
        <Card title="Recent work & expenses" actions={<Link className="btn ghost sm" to={`/vehicles/${v.id}/service`}>All {plural(s.records, 'record')}</Link>}>
          {ov.recent_records.length === 0 ? <Empty title="Nothing logged yet" /> : <div className="inline-list">{ov.recent_records.slice(0, 6).map(r => <div key={r.id} className="li"><Icon name={LOG_KIND_ICON[r.kind] || 'wrench'} size={15} title={KIND_LABEL[r.kind]} /><span className="grow"><b>{r.description}</b>{r.vendor && <span className="muted"> · {r.vendor}</span>}</span><span className="muted small">{fmtDate(r.date, false)}</span>{r.cost > 0 && <Money minor={r.cost} code={r.currency} />}</div>)}</div>}
          {s.last_service && <p className="muted small mt-sm">Last service {fmtDate(s.last_service.date)}{s.last_service.odometer ? ` at ${fmtOdo(s.last_service.odometer, v.odometer_unit)}` : ''}{s.last_service.odometer ? ` · ${fmtNum(s.odometer_estimate - s.last_service.odometer)} ${v.odometer_unit} ago` : ''}</p>}
        </Card>
      </div>

      <div className="overview-grid">
        {(ov.pinned_notes.length > 0 || ov.open_plans > 0) && <Card title="Pinned">
          {ov.pinned_notes.map(n => <div key={n.id} className="note-card pinned mb-sm"><b>{n.title}</b><pre>{n.body}</pre></div>)}
          {ov.open_plans > 0 && <p className="muted small"><Link to={`/vehicles/${v.id}/planner`}>{plural(ov.open_plans, 'open plan')}</Link> on the board.</p>}
        </Card>}
        {ov.trips.length > 0 && <Card title="Trips (Pocket Travel)">
          <div className="inline-list">{ov.trips.slice(0, 6).map(t => <div key={t.id} className="li"><span className="sw" style={{ width: 10, height: 10, borderRadius: 5, background: t.color || 'var(--accent)' }} /><span className="grow">{t.link ? <a href={t.link} target="_blank" rel="noreferrer"><b>{t.name}</b></a> : <b>{t.name}</b>}<span className="muted small"> · {fmtDate(t.start_date, false)} – {fmtDate(t.end_date, false)}</span></span><span className="muted small">{t.km > 0 && `${fmtNum(t.km)} ${v.odometer_unit} · `}{t.fills} fills</span><Money minor={t.fuel_cost + t.other_cost} code={v.currency} /></div>)}</div>
        </Card>}
        <Card title="Activity">
          {ov.activity.length === 0 ? <p className="muted">Changes show up here.</p> : <div className="inline-list">{ov.activity.map(a => <div key={a.id} className="li small"><span className="grow"><b>{a.action}</b>{a.subject && <> — {a.subject}</>}</span><span className="muted">{a.user_name && `${a.user_name} · `}{fmtDateTime(a.created_at, false)}</span></div>)}</div>}
        </Card>
      </div>
    </>
  )
}
