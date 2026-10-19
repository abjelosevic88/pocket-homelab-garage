import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { decimalsOf, todayISO } from '../lib/format'
import { fromVol, toVol, VOL_LABEL } from '../lib/units'
import { KIND_LABEL, STAGE_LABEL, type Fuel, type Note, type OdometerReading, type Part, type Plan, type Rec, type Reminder, type Vehicle } from '../lib/types'
import { Field, Modal, Segmented, useToast } from './ui'
import { Select } from './Select'
import { AmountInput, CurrencySelect, MoneyInput } from './Money'
import { MoneyBook, MoneyLinked, moneyBody, moneyWarning, useMoneyStatus, type MoneyChoice } from './MoneyBook'

const cap = (s: string) => s ? s[0].toUpperCase() + s.slice(1).replace(/_/g, ' ') : ''

function useSuggestions(vehicleId: number) {
  const [s, setS] = useState<{ categories: string[]; vendors: string[]; stations: string[] }>({ categories: [], vendors: [], stations: [] })
  useEffect(() => { api.get<typeof s>(`${V1}/vehicles/${vehicleId}/suggestions`).then(setS).catch(() => {}) }, [vehicleId])
  return s
}

function DataList({ id, items }: { id: string; items: string[] }) { return <datalist id={id}>{items.map(i => <option key={i} value={i} />)}</datalist> }

// ---------- vehicle ----------

export function VehicleModal({ vehicle, onClose, onSaved }: { vehicle?: Vehicle; onClose: () => void; onSaved: (v: Vehicle) => void }) {
  const { meta, settings } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({
    name: vehicle?.name || '', year: vehicle?.year || new Date().getFullYear(), make: vehicle?.make || '', model: vehicle?.model || '', trim: vehicle?.trim || '', plate: vehicle?.plate || '', vin: vehicle?.vin || '',
    kind: vehicle?.kind || 'car', fuel_type: vehicle?.fuel_type || 'petrol', odometer_unit: vehicle?.odometer_unit || settings?.distance_unit || 'km', tank_capacity: vehicle?.tank_capacity || 0,
    currency: vehicle?.currency || settings?.home_currency || 'EUR', color: vehicle?.color || '#c2410c', purchase_date: vehicle?.purchase_date || '', purchase_price: vehicle?.purchase_price || 0, purchase_odo: vehicle?.purchase_odo || 0,
    sold_date: vehicle?.sold_date || '', sold_price: vehicle?.sold_price || 0, status: vehicle?.status || 'active', notes: vehicle?.notes || '', tags: vehicle?.tags || '', initial_odometer: 0,
  })
  const [busy, setBusy] = useState(false)
  const set = (k: string, v: unknown) => setF(x => ({ ...x, [k]: v }))
  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { const v = vehicle ? await api.put<Vehicle>(`${V1}/vehicles/${vehicle.id}`, f) : await api.post<Vehicle>(`${V1}/vehicles`, f); toast(vehicle ? 'Vehicle saved' : 'Vehicle added'); onSaved(v) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={vehicle ? 'Edit vehicle' : 'Add a vehicle'} onClose={onClose} size="lg">
      <form onSubmit={submit} className="grid" style={{ gap: 14 }}>
        <div className="three-col tight">
          <Field label="Year"><input type="number" value={f.year || ''} onChange={e => set('year', Number(e.target.value))} min={1886} max={2100} /></Field>
          <Field label="Make"><input value={f.make} onChange={e => set('make', e.target.value)} placeholder="Škoda" autoFocus={!vehicle} /></Field>
          <Field label="Model"><input value={f.model} onChange={e => set('model', e.target.value)} placeholder="Fabia" /></Field>
        </div>
        <div className="three-col tight">
          <Field label="Nickname (optional)"><input value={f.name} onChange={e => set('name', e.target.value)} placeholder="The red one" /></Field>
          <Field label="Trim / engine"><input value={f.trim} onChange={e => set('trim', e.target.value)} placeholder="1.6 TDI" /></Field>
          <Field label="Plate"><input value={f.plate} onChange={e => set('plate', e.target.value)} /></Field>
        </div>
        <div className="three-col tight">
          <Field label="Type"><Select value={f.kind} onChange={v => set('kind', v)} options={(meta?.vehicle_kinds || []).map(k => ({ value: k, label: cap(k) }))} /></Field>
          <Field label="Fuel"><Select value={f.fuel_type} onChange={v => set('fuel_type', v)} options={(meta?.fuel_types || []).map(k => ({ value: k, label: cap(k) }))} /></Field>
          <Field group label="Odometer"><Segmented value={f.odometer_unit} onChange={v => set('odometer_unit', v)} options={[{ value: 'km', label: 'km' }, { value: 'mi', label: 'miles' }, { value: 'h', label: 'hours' }]} size="sm" /></Field>
        </div>
        <div className="three-col tight">
          <Field label="Currency" help="Every cost converts into this at the day's rate."><CurrencySelect value={f.currency} onChange={c => set('currency', c)} /></Field>
          <Field label={`Tank (${f.fuel_type === 'electric' ? 'kWh' : 'L'})`}><input type="number" step="0.1" value={f.tank_capacity || ''} onChange={e => set('tank_capacity', Number(e.target.value))} /></Field>
          <Field label="Colour"><input type="color" value={f.color} onChange={e => set('color', e.target.value)} style={{ height: 38, padding: 2 }} /></Field>
        </div>
        <div className="three-col tight">
          <Field label="Bought on"><input type="date" value={f.purchase_date} onChange={e => set('purchase_date', e.target.value)} /></Field>
          <Field label="Price paid"><MoneyInput value={f.purchase_price} code={f.currency} onChange={m => set('purchase_price', m || 0)} /></Field>
          <Field label={`Odometer at purchase (${f.odometer_unit})`}><input type="number" value={f.purchase_odo || ''} onChange={e => set('purchase_odo', Number(e.target.value))} /></Field>
        </div>
        {!vehicle && <Field label={`Current odometer (${f.odometer_unit})`} help="Logged as the first reading."><input type="number" value={f.initial_odometer || ''} onChange={e => set('initial_odometer', Number(e.target.value))} /></Field>}
        {vehicle && <div className="three-col tight">
          <Field group label="Status"><Segmented value={f.status} onChange={v => set('status', v)} options={[{ value: 'active', label: 'Active' }, { value: 'sold', label: 'Sold' }, { value: 'archived', label: 'Archived' }]} size="sm" /></Field>
          <Field label="Sold on"><input type="date" value={f.sold_date} onChange={e => set('sold_date', e.target.value)} /></Field>
          <Field label="Sold for"><MoneyInput value={f.sold_price} code={f.currency} onChange={m => set('sold_price', m || 0)} /></Field>
        </div>}
        <div className="two-col tight">
          <Field label="VIN"><input value={f.vin} onChange={e => set('vin', e.target.value)} className="mono" /></Field>
          <Field label="Tags"><input value={f.tags} onChange={e => set('tags', e.target.value)} placeholder="family daily" /></Field>
        </div>
        <Field label="Notes"><textarea rows={2} value={f.notes} onChange={e => set('notes', e.target.value)} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{vehicle ? 'Save' : 'Add vehicle'}</button></div>
      </form>
    </Modal>
  )
}

// ---------- fuel ----------

export function FuelModal({ vehicle, fuel, onClose, onSaved }: { vehicle: Vehicle; fuel?: Fuel; onClose: () => void; onSaved: (f: Fuel) => void }) {
  const { settings, currencies } = useApp()
  const { toast } = useToast()
  const sug = useSuggestions(vehicle.id)
  const u = settings!
  const [f, setF] = useState({
    date: fuel?.date || todayISO(), odometer: fuel?.odometer || (vehicle.summary ? Math.round(vehicle.summary.odometer_estimate) : 0), volume: fuel ? Number(toVol(fuel.volume, u).toFixed(3)) : 0,
    cost: fuel?.cost ?? null as number | null, currency: fuel?.currency || vehicle.currency, full_tank: fuel?.full_tank ?? true, missed: fuel?.missed ?? false, station: fuel?.station || '', grade: fuel?.grade || '', notes: fuel?.notes || '', tags: fuel?.tags || '',
    soc_start: fuel?.soc_start || 0, soc_end: fuel?.soc_end || 0,
  })
  const [busy, setBusy] = useState(false)
  const ms = useMoneyStatus(vehicle.id)
  const [mc, setMc] = useState<MoneyChoice | null>(null)
  const set = (k: string, v: unknown) => setF(x => ({ ...x, [k]: v }))
  const dec = decimalsOf(f.currency, currencies)
  const pricePer = f.volume > 0 && f.cost ? (f.cost / Math.pow(10, dec)) / f.volume : 0
  const ev = vehicle.fuel_type === 'electric'
  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    const body = { ...f, volume: fromVol(f.volume, u), cost: f.cost || 0, ...moneyBody(fuel?.money_tx_id ? null : mc) }
    try { const r = fuel ? await api.put<Fuel>(`${V1}/vehicles/${vehicle.id}/fuel/${fuel.id}`, body) : await api.post<Fuel>(`${V1}/vehicles/${vehicle.id}/fuel`, body); toast(fuel ? 'Fill-up saved' : 'Fill-up logged'); moneyWarning(r, toast); onSaved(r) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={fuel ? 'Edit fill-up' : ev ? 'Log a charge' : 'Log a fill-up'} onClose={onClose}>
      <form onSubmit={submit} className="grid" style={{ gap: 14 }}>
        <div className="two-col tight">
          <Field label="Date"><input type="date" value={f.date} onChange={e => set('date', e.target.value)} required /></Field>
          <Field label={`Odometer (${vehicle.odometer_unit})`} help={vehicle.summary && vehicle.summary.estimate_km > 0 ? `Dawarich estimate ≈ ${Math.round(vehicle.summary.odometer_estimate).toLocaleString()}` : undefined}><input type="number" inputMode="numeric" value={f.odometer || ''} onChange={e => set('odometer', Number(e.target.value))} required autoFocus /></Field>
        </div>
        <div className="two-col tight">
          <Field label={ev ? 'Energy (kWh)' : `Volume (${VOL_LABEL[u.volume_unit]})`}><input type="number" step="0.001" inputMode="decimal" value={f.volume || ''} onChange={e => set('volume', Number(e.target.value))} required /></Field>
          <Field label="Total cost" help={pricePer ? `${pricePer.toFixed(3)} ${f.currency}/${ev ? 'kWh' : VOL_LABEL[u.volume_unit]}` : undefined}><AmountInput amount={f.cost} currency={f.currency} onAmount={m => set('cost', m)} onCurrency={c => set('currency', c)} /></Field>
        </div>
        <div className="row" style={{ gap: 16, flexWrap: 'wrap' }}>
          <label className="row"><input type="checkbox" checked={f.full_tank} onChange={e => set('full_tank', e.target.checked)} style={{ width: 'auto' }} /> {ev ? 'Charged to 100 %' : 'Filled to full'}</label>
          <label className="row"><input type="checkbox" checked={f.missed} onChange={e => set('missed', e.target.checked)} style={{ width: 'auto' }} /> I skipped logging a fill-up before this one</label>
        </div>
        <div className="two-col tight">
          <Field label="Station"><input list="stations" value={f.station} onChange={e => set('station', e.target.value)} placeholder="Shell Centar" /><DataList id="stations" items={sug.stations} /></Field>
          <Field label={ev ? 'Charger type' : 'Grade'}><input value={f.grade} onChange={e => set('grade', e.target.value)} placeholder={ev ? 'AC 11 kW' : vehicle.fuel_type === 'diesel' ? 'diesel' : '95'} /></Field>
        </div>
        {ev && <div className="two-col tight">
          <Field label="SoC start %"><input type="number" value={f.soc_start || ''} onChange={e => set('soc_start', Number(e.target.value))} /></Field>
          <Field label="SoC end %"><input type="number" value={f.soc_end || ''} onChange={e => set('soc_end', Number(e.target.value))} /></Field>
        </div>}
        <Field label="Notes"><input value={f.notes} onChange={e => set('notes', e.target.value)} /></Field>
        {fuel?.money_tx_id ? <MoneyLinked status={ms} /> : <MoneyBook status={ms} value={mc} onChange={setMc} hasCost={!!f.cost} initial={!fuel} />}
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{fuel ? 'Save' : 'Log it'}</button></div>
      </form>
    </Modal>
  )
}

// ---------- record ----------

export function RecordModal({ vehicle, record, kind = 'service', onClose, onSaved }: { vehicle: Vehicle; record?: Rec; kind?: string; onClose: () => void; onSaved: (r: Rec) => void }) {
  const { meta, settings, currencies } = useApp()
  const { toast } = useToast()
  const sug = useSuggestions(vehicle.id)
  const [f, setF] = useState({
    kind: record?.kind || kind, category: record?.category || '', date: record?.date || todayISO(), odometer: record?.odometer ?? (vehicle.summary ? Math.round(vehicle.summary.odometer_estimate) : 0),
    description: record?.description || '', notes: record?.notes || '', vendor: record?.vendor || '', cost: record?.cost ?? null as number | null, currency: record?.currency || vehicle.currency, tags: record?.tags || '',
    parts: (record?.parts || []) as Part[],
  })
  const [busy, setBusy] = useState(false)
  const ms = useMoneyStatus(vehicle.id)
  const [mc, setMc] = useState<MoneyChoice | null>(null)
  const [inv, setInv] = useState<{ items: { id: number; name: string; quantity: number; unit: string }[]; url: string } | null>(null)
  const [invQ, setInvQ] = useState('')
  const set = (k: string, v: unknown) => setF(x => ({ ...x, [k]: v }))
  const cats = f.kind === 'expense' ? meta?.expense_categories || [] : meta?.service_categories || []
  const dec = decimalsOf(f.currency, currencies)
  const partsTotal = f.parts.reduce((a, p) => a + (p.cost || 0), 0)
  useEffect(() => {
    if (!settings?.inventory_configured || invQ.trim().length < 2) { setInv(null); return }
    const t = setTimeout(() => api.get<typeof inv>(`${V1}/inventory/search?q=${encodeURIComponent(invQ)}`).then(setInv).catch(() => setInv(null)), 300)
    return () => clearTimeout(t)
  }, [invQ, settings?.inventory_configured])
  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    const body = { ...f, cost: f.cost || 0, odometer: f.kind === 'expense' ? null : f.odometer || null, parts: f.parts.filter(p => p.name.trim()), ...moneyBody(record?.money_tx_id ? null : mc) }
    try { const r = record ? await api.put<Rec>(`${V1}/vehicles/${vehicle.id}/records/${record.id}`, body) : await api.post<Rec>(`${V1}/vehicles/${vehicle.id}/records`, body); toast(record ? 'Saved' : `${KIND_LABEL[r.kind]} logged`); moneyWarning(r, toast); if (r.inventory_error) toast(`Saved, but ${r.inventory_error}`, 'err'); onSaved(r) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const setPart = (i: number, patch: Partial<Part>) => set('parts', f.parts.map((p, j) => j === i ? { ...p, ...patch } : p))
  return (
    <Modal title={record ? `Edit ${KIND_LABEL[record.kind].toLowerCase()}` : 'Log work or an expense'} onClose={onClose} size="lg">
      <form onSubmit={submit} className="grid" style={{ gap: 14 }}>
        <Field group label="Kind"><Segmented value={f.kind} onChange={v => set('kind', v)} options={(meta?.record_kinds || []).map(k => ({ value: k, label: KIND_LABEL[k] }))} size="sm" /></Field>
        <Field label="What was done"><input value={f.description} onChange={e => set('description', e.target.value)} required autoFocus placeholder={f.kind === 'expense' ? 'Registration' : 'Oil & filter change'} /></Field>
        <div className="three-col tight">
          <Field label="Date"><input type="date" value={f.date} onChange={e => set('date', e.target.value)} required /></Field>
          {f.kind !== 'expense' ? <Field label={`Odometer (${vehicle.odometer_unit})`}><input type="number" inputMode="numeric" value={f.odometer || ''} onChange={e => set('odometer', Number(e.target.value))} /></Field> : <span />}
          <Field label="Category"><input list="cats" value={f.category} onChange={e => set('category', e.target.value)} placeholder={f.kind === 'expense' ? 'insurance' : 'oil'} /><DataList id="cats" items={Array.from(new Set([...cats, ...sug.categories]))} /></Field>
        </div>
        <div className="two-col tight">
          <Field label="Total cost" help={partsTotal > 0 && f.cost != null && f.cost < partsTotal ? 'Lower than the parts listed below' : undefined}><AmountInput amount={f.cost} currency={f.currency} onAmount={m => set('cost', m)} onCurrency={c => set('currency', c)} /></Field>
          <Field label={f.kind === 'expense' ? 'Paid to' : 'Workshop / vendor'}><input list="vendors" value={f.vendor} onChange={e => set('vendor', e.target.value)} /><DataList id="vendors" items={sug.vendors} /></Field>
        </div>
        {f.kind !== 'expense' && <Field group label={<span>Parts & fluids <span className="muted small">— what was fitted{settings?.inventory_configured ? ', pick from Pocket Inventory or type' : ''}</span></span>}>
          <div className="parts">
            {f.parts.map((p, i) => (
              <div key={i} className="part">
                <span className="part-name"><input value={p.name} onChange={e => { const hit = inv?.items.find(it => it.name === e.target.value); setPart(i, { name: e.target.value, inventory_id: hit?.id }); setInvQ(e.target.value) }} placeholder="Oil filter" list={settings?.inventory_configured ? `inv-${i}` : undefined} />
                  {p.inventory_id ? <span className="part-inv" title={settings?.inventory_take ? 'From Pocket Inventory: taken off the shelf when you save' : 'From Pocket Inventory'}>📦</span> : null}</span>
                {settings?.inventory_configured && <datalist id={`inv-${i}`}>{(inv?.items || []).map(it => <option key={it.id} value={it.name}>{`${it.quantity} ${it.unit || ''} on hand`}</option>)}</datalist>}
                <input type="number" step="0.01" value={p.qty || ''} onChange={e => setPart(i, { qty: Number(e.target.value) })} placeholder="qty" />
                <input className="partno" value={p.part_no} onChange={e => setPart(i, { part_no: e.target.value })} placeholder="part no." />
                <MoneyInput value={p.cost} code={f.currency} onChange={m => setPart(i, { cost: m || 0 })} placeholder="cost" />
                <button type="button" className="btn ghost sm" onClick={() => set('parts', f.parts.filter((_, j) => j !== i))} aria-label="Remove">✕</button>
              </div>
            ))}
            <div><button type="button" className="btn sm" onClick={() => set('parts', [...f.parts, { name: '', qty: 1, cost: 0, part_no: '' }])}>+ Add part</button>{partsTotal > 0 && <span className="muted small" style={{ marginLeft: 10 }}>parts {(partsTotal / Math.pow(10, dec)).toFixed(dec)} {f.currency}</span>}</div>
          </div>
        </Field>}
        <div className="two-col tight">
          <Field label="Notes"><textarea rows={2} value={f.notes} onChange={e => set('notes', e.target.value)} /></Field>
          <Field label="Tags"><input value={f.tags} onChange={e => set('tags', e.target.value)} placeholder="warranty diy" /></Field>
        </div>
        {record?.money_tx_id ? <MoneyLinked status={ms} /> : <MoneyBook status={ms} value={mc} onChange={setMc} hasCost={!!f.cost} initial={!record} />}
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{record ? 'Save' : 'Log it'}</button></div>
      </form>
    </Modal>
  )
}

// ---------- odometer ----------

export function OdometerModal({ vehicle, reading, onClose, onSaved }: { vehicle: Vehicle; reading?: OdometerReading; onClose: () => void; onSaved: (o: OdometerReading) => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ date: reading?.date || todayISO(), reading: reading?.reading || (vehicle.summary ? Math.round(vehicle.summary.odometer_estimate) : 0), notes: reading?.notes || '' })
  const [busy, setBusy] = useState(false)
  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { const o = reading ? await api.put<OdometerReading>(`${V1}/vehicles/${vehicle.id}/odometer/${reading.id}`, f) : await api.post<OdometerReading>(`${V1}/vehicles/${vehicle.id}/odometer`, f); toast('Reading saved'); onSaved(o) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={reading ? 'Edit reading' : 'Odometer reading'} onClose={onClose}>
      <form onSubmit={submit} className="grid" style={{ gap: 14 }}>
        <div className="two-col tight">
          <Field label="Date"><input type="date" value={f.date} onChange={e => setF({ ...f, date: e.target.value })} required /></Field>
          <Field label={`Reading (${vehicle.odometer_unit})`}><input type="number" inputMode="numeric" value={f.reading || ''} onChange={e => setF({ ...f, reading: Number(e.target.value) })} required autoFocus /></Field>
        </div>
        <Field label="Note"><input value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} placeholder="Before the trip" /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>Save</button></div>
      </form>
    </Modal>
  )
}

// ---------- reminder ----------

export function ReminderModal({ vehicle, reminder, onClose, onSaved }: { vehicle: Vehicle; reminder?: Reminder; onClose: () => void; onSaved: (r: Reminder) => void }) {
  const { meta } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({
    title: reminder?.title || '', notes: reminder?.notes || '', kind: reminder?.kind || 'service', category: reminder?.category || '', due_date: reminder?.due_date || '', due_odometer: reminder?.due_odometer || 0,
    interval_days: reminder?.interval_days || 0, interval_km: reminder?.interval_km || 0, soon_days: reminder?.soon_days || 14, soon_km: reminder?.soon_km || 500,
  })
  const [busy, setBusy] = useState(false)
  const set = (k: string, v: unknown) => setF(x => ({ ...x, [k]: v }))
  const applyTemplate = (t: string) => {
    const tpl = meta?.reminder_templates.find(x => x.title === t)
    if (!tpl) return
    const est = vehicle.summary?.odometer_estimate || 0
    const d = new Date(); d.setDate(d.getDate() + tpl.interval_days)
    setF(x => ({ ...x, title: tpl.title, kind: tpl.kind, category: tpl.category, interval_days: tpl.interval_days, interval_km: tpl.interval_km, due_date: tpl.interval_days ? d.toISOString().slice(0, 10) : x.due_date, due_odometer: tpl.interval_km && est ? Math.round(est + tpl.interval_km) : x.due_odometer }))
  }
  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    const body = { ...f, due_odometer: f.due_odometer || null }
    try { const r = reminder ? await api.put<Reminder>(`${V1}/vehicles/${vehicle.id}/reminders/${reminder.id}`, body) : await api.post<Reminder>(`${V1}/vehicles/${vehicle.id}/reminders`, body); toast('Reminder saved'); onSaved(r) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={reminder ? 'Edit reminder' : 'New reminder'} onClose={onClose}>
      <form onSubmit={submit} className="grid" style={{ gap: 14 }}>
        {!reminder && meta && meta.reminder_templates.length > 0 && <Field label="Start from a template"><Select value="" onChange={applyTemplate} options={meta.reminder_templates.map(t => ({ value: t.title, label: t.title, hint: [t.interval_days ? `every ${t.interval_days} d` : '', t.interval_km ? `every ${t.interval_km} km` : ''].filter(Boolean).join(' / ') }))} placeholder="Pick one, or write your own below" /></Field>}
        <Field label="Title"><input value={f.title} onChange={e => set('title', e.target.value)} required autoFocus placeholder="Oil change" /></Field>
        <div className="two-col tight">
          <Field label="Due date"><input type="date" value={f.due_date} onChange={e => set('due_date', e.target.value)} /></Field>
          <Field label={`Due at odometer (${vehicle.odometer_unit})`} help={vehicle.summary ? `now ≈ ${Math.round(vehicle.summary.odometer_estimate).toLocaleString()}` : undefined}><input type="number" value={f.due_odometer || ''} onChange={e => set('due_odometer', Number(e.target.value))} /></Field>
        </div>
        <div className="two-col tight">
          <Field label="Repeat every … days" help="0 = once"><input type="number" value={f.interval_days || ''} onChange={e => set('interval_days', Number(e.target.value))} /></Field>
          <Field label={`Repeat every … ${vehicle.odometer_unit}`}><input type="number" value={f.interval_km || ''} onChange={e => set('interval_km', Number(e.target.value))} /></Field>
        </div>
        <div className="two-col tight">
          <Field label="When done, log as"><Select value={f.kind} onChange={v => set('kind', v)} options={[...(meta?.record_kinds || []).map(k => ({ value: k, label: KIND_LABEL[k] })), { value: 'none', label: 'Nothing (just a reminder)' }]} /></Field>
          <Field label="Category"><input value={f.category} onChange={e => set('category', e.target.value)} placeholder="oil" list="rcats" /><DataList id="rcats" items={[...(meta?.service_categories || []), ...(meta?.expense_categories || [])]} /></Field>
        </div>
        <div className="two-col tight">
          <Field label="Warn … days ahead"><input type="number" value={f.soon_days} onChange={e => set('soon_days', Number(e.target.value))} /></Field>
          <Field label={`Warn … ${vehicle.odometer_unit} ahead`}><input type="number" value={f.soon_km} onChange={e => set('soon_km', Number(e.target.value))} /></Field>
        </div>
        <Field label="Notes"><textarea rows={2} value={f.notes} onChange={e => set('notes', e.target.value)} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>Save</button></div>
      </form>
    </Modal>
  )
}

export function DoneModal({ vehicle, reminder, onClose, onDone }: { vehicle: Vehicle; reminder: Reminder; onClose: () => void; onDone: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ date: todayISO(), odometer: vehicle.summary ? Math.round(vehicle.summary.odometer_estimate) : 0, log_record: reminder.kind !== 'none', cost: null as number | null, currency: vehicle.currency, vendor: '', notes: '' })
  const [busy, setBusy] = useState(false)
  const ms = useMoneyStatus(vehicle.id)
  const [mc, setMc] = useState<MoneyChoice | null>(null)
  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { const r = await api.post<{ record: Rec | null }>(`${V1}/vehicles/${vehicle.id}/reminders/${reminder.id}/done`, { ...f, cost: f.cost || 0, odometer: f.odometer || null, ...moneyBody(mc) }); toast(reminder.interval_days || reminder.interval_km ? 'Done — rolled forward' : 'Done'); if (r.record) moneyWarning(r.record, toast); onDone() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={`Done: ${reminder.title}`} onClose={onClose}>
      <form onSubmit={submit} className="grid" style={{ gap: 14 }}>
        <div className="two-col tight">
          <Field label="Done on"><input type="date" value={f.date} onChange={e => setF({ ...f, date: e.target.value })} required /></Field>
          <Field label={`Odometer (${vehicle.odometer_unit})`}><input type="number" value={f.odometer || ''} onChange={e => setF({ ...f, odometer: Number(e.target.value) })} /></Field>
        </div>
        {reminder.kind !== 'none' && <label className="row"><input type="checkbox" checked={f.log_record} onChange={e => setF({ ...f, log_record: e.target.checked })} style={{ width: 'auto' }} /> Log it as a {KIND_LABEL[reminder.kind]?.toLowerCase()} record</label>}
        {f.log_record && <>
          <div className="two-col tight">
            <Field label="Cost"><AmountInput amount={f.cost} currency={f.currency} onAmount={m => setF({ ...f, cost: m })} onCurrency={c => setF({ ...f, currency: c })} /></Field>
            <Field label="Workshop"><input value={f.vendor} onChange={e => setF({ ...f, vendor: e.target.value })} /></Field>
          </div>
          <Field label="Notes"><input value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} /></Field>
          <MoneyBook status={ms} value={mc} onChange={setMc} hasCost={!!f.cost} />
        </>}
        {(reminder.interval_days > 0 || reminder.interval_km > 0) && <p className="muted small">Recurring: the next due point is set from this date and odometer.</p>}
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>Mark done</button></div>
      </form>
    </Modal>
  )
}

// ---------- plan ----------

export function PlanModal({ vehicle, plan, stage, onClose, onSaved }: { vehicle: Vehicle; plan?: Plan; stage?: string; onClose: () => void; onSaved: (p: Plan) => void }) {
  const { meta } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ title: plan?.title || '', notes: plan?.notes || '', kind: plan?.kind || 'repair', category: plan?.category || '', priority: plan?.priority || 'normal', stage: plan?.stage || stage || 'backlog', estimate: plan?.estimate ?? null as number | null, target_date: plan?.target_date || '' })
  const [busy, setBusy] = useState(false)
  const set = (k: string, v: unknown) => setF(x => ({ ...x, [k]: v }))
  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { const p = plan ? await api.put<Plan>(`${V1}/vehicles/${vehicle.id}/plans/${plan.id}`, { ...f, estimate: f.estimate || 0 }) : await api.post<Plan>(`${V1}/vehicles/${vehicle.id}/plans`, { ...f, estimate: f.estimate || 0 }); toast('Saved'); onSaved(p) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={plan ? 'Edit plan' : 'Plan some work'} onClose={onClose}>
      <form onSubmit={submit} className="grid" style={{ gap: 14 }}>
        <Field label="What"><input value={f.title} onChange={e => set('title', e.target.value)} required autoFocus placeholder="Replace rear wiper" /></Field>
        <div className="two-col tight">
          <Field label="Kind"><Select value={f.kind} onChange={v => set('kind', v)} options={(meta?.record_kinds || []).map(k => ({ value: k, label: KIND_LABEL[k] }))} /></Field>
          <Field label="Priority"><Select value={f.priority} onChange={v => set('priority', v)} options={(meta?.priorities || []).map(k => ({ value: k, label: cap(k) }))} /></Field>
        </div>
        <div className="two-col tight">
          <Field label="Stage"><Select value={f.stage} onChange={v => set('stage', v)} options={(meta?.plan_stages || []).map(k => ({ value: k, label: STAGE_LABEL[k] }))} /></Field>
          <Field label="Estimate"><MoneyInput value={f.estimate} code={vehicle.currency} onChange={m => set('estimate', m)} /></Field>
        </div>
        <div className="two-col tight">
          <Field label="Category"><input value={f.category} onChange={e => set('category', e.target.value)} list="pcats" /><DataList id="pcats" items={[...(meta?.service_categories || []), ...(meta?.expense_categories || [])]} /></Field>
          <Field label="Target date"><input type="date" value={f.target_date} onChange={e => set('target_date', e.target.value)} /></Field>
        </div>
        <Field label="Notes"><textarea rows={3} value={f.notes} onChange={e => set('notes', e.target.value)} placeholder="Part numbers, links, how-to…" /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>Save</button></div>
      </form>
    </Modal>
  )
}

export function PlanDoneModal({ vehicle, plan, onClose, onDone }: { vehicle: Vehicle; plan: Plan; onClose: () => void; onDone: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ date: todayISO(), odometer: vehicle.summary ? Math.round(vehicle.summary.odometer_estimate) : 0, cost: plan.estimate || null as number | null, currency: vehicle.currency, vendor: '', notes: '' })
  const [busy, setBusy] = useState(false)
  const ms = useMoneyStatus(vehicle.id)
  const [mc, setMc] = useState<MoneyChoice | null>(null)
  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { const r = await api.post<{ record: Rec }>(`${V1}/vehicles/${vehicle.id}/plans/${plan.id}/done`, { ...f, cost: f.cost || 0, odometer: f.odometer || null, ...moneyBody(mc) }); toast('Logged and moved to done'); moneyWarning(r.record, toast); onDone() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={`Done: ${plan.title}`} onClose={onClose}>
      <form onSubmit={submit} className="grid" style={{ gap: 14 }}>
        <p className="muted small">This logs a {KIND_LABEL[plan.kind]?.toLowerCase()} record with the real cost and moves the card to Done.</p>
        <div className="two-col tight">
          <Field label="Date"><input type="date" value={f.date} onChange={e => setF({ ...f, date: e.target.value })} required /></Field>
          <Field label={`Odometer (${vehicle.odometer_unit})`}><input type="number" value={f.odometer || ''} onChange={e => setF({ ...f, odometer: Number(e.target.value) })} /></Field>
        </div>
        <div className="two-col tight">
          <Field label="Actual cost"><AmountInput amount={f.cost} currency={f.currency} onAmount={m => setF({ ...f, cost: m })} onCurrency={c => setF({ ...f, currency: c })} /></Field>
          <Field label="Workshop"><input value={f.vendor} onChange={e => setF({ ...f, vendor: e.target.value })} /></Field>
        </div>
        <Field label="Notes"><input value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} /></Field>
        <MoneyBook status={ms} value={mc} onChange={setMc} hasCost={!!f.cost} />
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>Log & finish</button></div>
      </form>
    </Modal>
  )
}

// ---------- note ----------

export function NoteModal({ vehicle, note, onClose, onSaved }: { vehicle: Vehicle; note?: Note; onClose: () => void; onSaved: (n: Note) => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ title: note?.title || '', body: note?.body || '', pinned: note?.pinned || false })
  const [busy, setBusy] = useState(false)
  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { const n = note ? await api.put<Note>(`${V1}/vehicles/${vehicle.id}/notes/${note.id}`, f) : await api.post<Note>(`${V1}/vehicles/${vehicle.id}/notes`, f); toast('Note saved'); onSaved(n) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={note ? 'Edit note' : 'New note'} onClose={onClose}>
      <form onSubmit={submit} className="grid" style={{ gap: 14 }}>
        <Field label="Title"><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} autoFocus placeholder="Tyre pressures" /></Field>
        <Field label="Text"><textarea rows={6} value={f.body} onChange={e => setF({ ...f, body: e.target.value })} /></Field>
        <label className="row"><input type="checkbox" checked={f.pinned} onChange={e => setF({ ...f, pinned: e.target.checked })} style={{ width: 'auto' }} /> Pin to the overview</label>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>Save</button></div>
      </form>
    </Modal>
  )
}
