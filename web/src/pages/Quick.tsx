import { useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Fuel, Vehicle } from '../lib/types'
import { decimalsOf, todayISO } from '../lib/format'
import { fromVol, VOL_LABEL } from '../lib/units'
import { Field, Loading, useToast } from '../components/ui'
import { Select } from '../components/Select'
import { AmountInput } from '../components/Money'
import { Icon } from '../components/icons'
import { OdometerModal, RecordModal } from '../components/forms'
import { MoneyBook, moneyBody, moneyWarning, useMoneyStatus, type MoneyChoice } from '../components/MoneyBook'

type Kind = 'fuel' | 'service' | 'expense' | 'odometer'

/** One-screen logging page for the phone: /quick?kind=fuel&vehicle=1. Also the PWA shortcut target. */
export default function Quick() {
  const { vehicles, settings, currencies, theme, toggleTheme } = useApp()
  const [params, setParams] = useSearchParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const active = vehicles.filter(v => v.status === 'active')
  const kind = (params.get('kind') as Kind) || 'fuel'
  const vid = Number(params.get('vehicle')) || Number(localStorage.getItem('pg-quick-vehicle')) || active[0]?.id || 0
  const [vehicle, setVehicle] = useState<Vehicle | null>(null)
  useEffect(() => { if (vid) api.get<Vehicle>(`${V1}/vehicles/${vid}`).then(setVehicle).catch(() => setVehicle(null)) }, [vid])
  useEffect(() => { if (vid) localStorage.setItem('pg-quick-vehicle', String(vid)) }, [vid])
  const setKind = (k: Kind) => { params.set('kind', k); setParams(params, { replace: true }) }
  const [f, setF] = useState({ odometer: 0, volume: 0, cost: null as number | null, currency: '', full_tank: true, station: '' })
  const [busy, setBusy] = useState(false)
  const [last, setLast] = useState<Fuel | null>(null)
  const ms = useMoneyStatus(vehicle?.id)
  const [mc, setMc] = useState<MoneyChoice | null>(null)
  useEffect(() => { setMc(null) }, [vehicle?.id])
  useEffect(() => { if (vehicle) setF(x => ({ ...x, odometer: x.odometer || Math.round(vehicle.summary?.odometer_estimate || 0), currency: x.currency || vehicle.currency, station: x.station || vehicle.summary?.last_fuel?.station || '' })) }, [vehicle])
  if (!settings) return <Loading />
  if (active.length === 0) return <div className="quick"><p className="muted">Add a vehicle first.</p><Link className="btn" to="/vehicles">Vehicles</Link></div>
  const u = settings
  const dec = decimalsOf(f.currency || 'EUR', currencies)
  const pricePer = f.volume > 0 && f.cost ? (f.cost / Math.pow(10, dec)) / f.volume : 0
  const submitFuel = async (e: React.FormEvent) => {
    e.preventDefault(); if (!vehicle) return; setBusy(true)
    try {
      const r = await api.post<Fuel>(`${V1}/vehicles/${vehicle.id}/fuel`, { date: todayISO(), odometer: f.odometer, volume: fromVol(f.volume, u), cost: f.cost || 0, currency: f.currency, full_tank: f.full_tank, station: f.station, ...moneyBody(mc) })
      setLast(r); toast('Fill-up logged'); moneyWarning(r, toast); setF(x => ({ ...x, volume: 0, cost: null }))
      api.get<Vehicle>(`${V1}/vehicles/${vehicle.id}`).then(setVehicle)
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <div className="quick">
      <div className="quick-head">
        <Link to="/" className="row" style={{ gap: 10, color: 'inherit', textDecoration: 'none' }}><img src="/favicon.svg" alt="" /><h1 style={{ fontSize: 18 }}>Quick log</h1></Link>
        <span className="spacer" />
        <button className="btn ghost icon-only" onClick={toggleTheme} aria-label="Toggle theme" title="Toggle theme"><Icon name={theme === 'dark' ? 'sun' : 'moon'} size={17} /></button>
        <Link className="btn sm" to="/">Open app</Link>
      </div>
      {active.length > 1 && <div className="mb"><Select value={String(vid)} onChange={v => { params.set('vehicle', v); setParams(params, { replace: true }) }} options={active.map(v => ({ value: String(v.id), label: v.display_name, hint: v.plate }))} /></div>}
      <div className="quick-kinds">
        {([['fuel', 'fuel', 'Fuel'], ['service', 'wrench', 'Work'], ['expense', 'receipt', 'Expense'], ['odometer', 'gauge', 'Odometer']] as [Kind, string, string][]).map(([k, i, l]) => <button key={k} type="button" className={kind === k ? 'active' : ''} onClick={() => setKind(k)}><Icon name={i} size={20} /><span>{l}</span></button>)}
      </div>
      {!vehicle ? <Loading /> : kind === 'fuel' ? (
        <form onSubmit={submitFuel} className="grid" style={{ gap: 14 }}>
          <div className="muted small">{vehicle.display_name}{vehicle.summary?.last_fuel && <> · last fill {vehicle.summary.last_fuel.volume.toFixed(1)} L at {vehicle.summary.last_fuel.odometer.toLocaleString()} {vehicle.odometer_unit}</>}</div>
          <Field label={`Odometer (${vehicle.odometer_unit})`} className="big-input" help={vehicle.summary && vehicle.summary.estimate_km > 0 ? `Location history says ≈ ${Math.round(vehicle.summary.odometer_estimate).toLocaleString()}` : undefined}><input type="number" inputMode="numeric" value={f.odometer || ''} onChange={e => setF({ ...f, odometer: Number(e.target.value) })} required autoFocus /></Field>
          <Field label={`Volume (${VOL_LABEL[u.volume_unit]})`} className="big-input"><input type="number" step="0.01" inputMode="decimal" value={f.volume || ''} onChange={e => setF({ ...f, volume: Number(e.target.value) })} required /></Field>
          <Field label="Total paid" className="big-input" help={pricePer ? `${pricePer.toFixed(3)} ${f.currency}/${VOL_LABEL[u.volume_unit]}` : undefined}><AmountInput amount={f.cost} currency={f.currency || vehicle.currency} onAmount={m => setF({ ...f, cost: m })} onCurrency={c => setF({ ...f, currency: c })} /></Field>
          <div className="two-col tight">
            <Field label="Station"><input value={f.station} onChange={e => setF({ ...f, station: e.target.value })} /></Field>
            <Field group label="Tank"><label className="row" style={{ paddingTop: 8 }}><input type="checkbox" checked={f.full_tank} onChange={e => setF({ ...f, full_tank: e.target.checked })} style={{ width: 'auto' }} /> filled to full</label></Field>
          </div>
          <MoneyBook status={ms} value={mc} onChange={setMc} hasCost={!!f.cost} />
          <button className="btn primary lg" disabled={busy}>{busy ? 'Saving…' : 'Log fill-up'}</button>
          {last && <div className="callout success">Logged {last.volume.toFixed(2)} L{last.distance > 0 && <> · {Math.round(last.distance)} {vehicle.odometer_unit} since the last one</>}{last.economy > 0 && <> · {last.economy.toFixed(2)} L/100</>}{last.money_tx_id && <> · booked in Pocket Money</>}. <Link to={`/vehicles/${vehicle.id}/fuel`}>See the list</Link></div>}
        </form>
      ) : kind === 'odometer' ? (
        <OdometerModal vehicle={vehicle} onClose={() => navigate(`/vehicles/${vehicle.id}/odometer`)} onSaved={() => navigate(`/vehicles/${vehicle.id}/odometer`)} />
      ) : (
        <RecordModal vehicle={vehicle} kind={kind} onClose={() => navigate(`/vehicles/${vehicle.id}/service`)} onSaved={() => navigate(`/vehicles/${vehicle.id}/service`)} />
      )}
    </div>
  )
}
