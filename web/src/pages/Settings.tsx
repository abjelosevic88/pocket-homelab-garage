import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, Settings as SettingsT, User } from '../lib/types'
import { DATE_FORMATS, TIME_FORMATS, fmtBytes, fmtDateTime, type DateFormat, type TimeFormat } from '../lib/format'
import { ECON_OPTIONS, VOL_OPTIONS } from '../lib/units'
import { Card, Confirm, Field, Loading, Modal, PageHeader, Segmented, Tabs, useAsync, useToast } from '../components/ui'
import { Select } from '../components/Select'
import { CurrencySelect } from '../components/Money'
import { Avatar } from '../components/people'
import { useMoneyStatus } from '../components/MoneyBook'

type Tab = 'general' | 'integrations' | 'import' | 'users' | 'profile' | 'api' | 'data' | 'about'

export default function Settings() {
  const [tab, setTab] = useState<Tab>(() => (new URLSearchParams(window.location.search).get('tab') as Tab) || 'general')
  return (
    <>
      <PageHeader title="Settings" />
      <Tabs tabs={[{ id: 'general', label: 'General' }, { id: 'integrations', label: 'Integrations' }, { id: 'import', label: 'Import' }, { id: 'users', label: 'Users' }, { id: 'profile', label: 'Profile' }, { id: 'api', label: 'API & apps' }, { id: 'data', label: 'Backup & data' }, { id: 'about', label: 'About' }]} value={tab} onChange={setTab} />
      <div className="mt">
        {tab === 'general' && <General />}
        {tab === 'integrations' && <Integrations />}
        {tab === 'import' && <Import />}
        {tab === 'users' && <Users />}
        {tab === 'profile' && <Profile />}
        {tab === 'api' && <Tokens />}
        {tab === 'data' && <Data />}
        {tab === 'about' && <About />}
      </div>
    </>
  )
}

function useSettingsForm() {
  const { settings, setSettings, user } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (patch?: Partial<SettingsT>) => {
    if (!f) return
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, { ...f, ...patch }); setSettings(s); setF(s); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return { f, setF, save, busy, isAdmin: user?.role === 'admin' }
}

function General() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  if (!f) return <Loading />
  return (
    <div className="two-col">
      <Card title="Your preferences">
        <div className="grid" style={{ gap: 14 }}>
          <Field label="App title"><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} maxLength={40} disabled={!isAdmin} /></Field>
          <Field label="Home currency" help="Totals across vehicles convert into this. Each vehicle has its own currency too."><CurrencySelect value={f.home_currency} onChange={c => setF({ ...f, home_currency: c })} /></Field>
          <div className="two-col tight">
            <Field label="Date format"><Select value={f.date_format} onChange={v => setF({ ...f, date_format: v as DateFormat })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></Field>
            <Field label="Time format"><Select value={f.time_format} onChange={v => setF({ ...f, time_format: v as TimeFormat })} options={TIME_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></Field>
          </div>
          <div className="three-col tight">
            <Field group label="New vehicles use"><Segmented value={f.distance_unit} onChange={v => setF({ ...f, distance_unit: v as 'km' | 'mi' })} options={[{ value: 'km', label: 'km' }, { value: 'mi', label: 'miles' }]} size="sm" /></Field>
            <Field label="Volume"><Select value={f.volume_unit} onChange={v => setF({ ...f, volume_unit: v as SettingsT['volume_unit'] })} options={VOL_OPTIONS.map(o => ({ value: o.id, label: o.label }))} /></Field>
            <Field label="Economy"><Select value={f.economy_unit} onChange={v => setF({ ...f, economy_unit: v as SettingsT['economy_unit'] })} options={ECON_OPTIONS.map(o => ({ value: o.id, label: o.label }))} /></Field>
          </div>
          <label className="row"><input type="checkbox" checked={f.fx_auto} onChange={e => setF({ ...f, fx_auto: e.target.checked })} style={{ width: 'auto' }} disabled={!isAdmin || f.fx_provider === 'none'} /> Look up exchange rates automatically (Frankfurter / open.er-api.com; euro pegs like BAM are built in){f.fx_provider === 'none' && <span className="muted small"> — disabled on the server</span>}</label>
          <div><button className="btn primary" onClick={() => save()} disabled={busy}>Save</button></div>
          <p className="muted small">Currency, formats and units are yours alone. {isAdmin ? 'The app title and exchange-rate lookups apply to everyone on this box.' : 'The app title and exchange-rate lookups are set by an admin for everyone.'}</p>
        </div>
      </Card>
      <Card title="How it fits together">
        <table className="table compact"><tbody>
          <tr><th>Fuel</th><td className="muted">One row per fill-up. Distance and economy come from the previous full tank; partial fills roll into the next full one.</td></tr>
          <tr><th>Service & costs</th><td className="muted">Service, repairs, upgrades and plain expenses in one list with a kind, a category and the parts fitted.</td></tr>
          <tr><th>Odometer</th><td className="muted">Every fill-up or record with a reading <i>is</i> the odometer log. Dawarich adds the kilometres driven since.</td></tr>
          <tr><th>Reminders</th><td className="muted">By date, by odometer or both. Recurring ones roll forward when done; logging matching work completes them.</td></tr>
          <tr><th>Pocket Money</th><td className="muted">Costs you log can become transactions in your Pocket Money (through Pocket Core, or a token). Edits and deletes follow them.</td></tr>
          <tr><th>Firefly</th><td className="muted">Tagged withdrawals link to what you logged; the rest become expenses. Rows can be pushed back as withdrawals.</td></tr>
          <tr><th>Phone & Raycast</th><td className="muted">The LubeLogger-shaped API lives at <code>/api/…</code>, so the Pocket Homelab app and the Raycast extension only need the new URL.</td></tr>
        </tbody></table>
      </Card>
    </div>
  )
}

function Integrations() {
  const { f, setF, save, busy } = useSettingsForm()
  const { toast } = useToast()
  const [test, setTest] = useState<Record<string, string>>({})
  const [accounts, setAccounts] = useState<{ id: string; name: string; currency: string }[]>([])
  const [cats, setCats] = useState<string[]>([])
  useEffect(() => { if (f?.firefly_configured) { api.get<typeof accounts>(`${V1}/firefly/accounts`).then(setAccounts).catch(() => {}); api.get<string[]>(`${V1}/firefly/categories`).then(setCats).catch(() => {}) } }, [f?.firefly_configured])
  if (!f) return <Loading />
  const run = async (which: string, url: string, key: string, extra: Record<string, string> = {}) => {
    setTest(t => ({ ...t, [which]: 'testing…' }))
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which, url, key, ...extra }); setTest(t => ({ ...t, [which]: r.ok ? `✓ ${r.message}` : `✕ ${r.error}` })) } catch (e) { setTest(t => ({ ...t, [which]: `✕ ${(e as Error).message}` })) }
  }
  const status = (which: string) => <span className={`small ${test[which]?.startsWith('✓') ? 'ok' : test[which]?.startsWith('✕') ? 'bad' : 'muted'}`}>{test[which]}</span>
  const viaBadge = (via: string) => <span className={`badge ${via ? 'cat' : ''}`}>{via === 'core' ? 'connected through Pocket Core' : via === 'token' ? 'connected with a token' : 'not connected'}</span>
  const badge = (on: boolean) => <span className={`badge ${on ? 'cat' : ''}`}>{on ? 'connected' : 'not set'}</span>
  const notifyRun = async () => { try { const r = await api.post<{ sent: number }>(`${V1}/notify/run?force=1`); toast(`${r.sent} message${r.sent === 1 ? '' : 's'} sent`) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <div className="grid" style={{ gap: 16 }}>
      <PocketMoneyCard f={f} setF={setF} run={run} status={status} />
      <Card title="💶 Firefly III" actions={badge(f.firefly_configured)}>
        <p className="muted small">What the car really costs. Tag the vehicle's withdrawals in Firefly (e.g. <code>skoda</code>) and set that tag on the vehicle. Rows matching a fill-up or record by date and amount are linked; the rest come in as expenses. Synced every 30 minutes and on demand.</p>
        <div className="grid" style={{ gap: 12 }}>
          <Field label="URL" help="Inside Docker use http://host.docker.internal:PORT or the service name."><input value={f.firefly_url} onChange={e => setF({ ...f, firefly_url: e.target.value })} placeholder="http://host.docker.internal:8083" /></Field>
          <Field label="Personal access token" help="Firefly → Options → Profile → OAuth → Personal access tokens. Stored on the server; shown masked."><input value={f.firefly_token} onChange={e => setF({ ...f, firefly_token: e.target.value })} autoComplete="off" /></Field>
          <div className="two-col tight">
            <Field label="Pay from (default account for pushed withdrawals)">{accounts.length > 0 ? <Select value={f.firefly_account} onChange={a => setF({ ...f, firefly_account: a })} options={accounts.map(a => ({ value: a.name, label: a.name, hint: a.currency }))} clearLabel="None" /> : <input value={f.firefly_account} onChange={e => setF({ ...f, firefly_account: e.target.value })} placeholder="Checking account" />}</Field>
            <Field label="Category for pushed withdrawals">{cats.length > 0 ? <Select value={f.firefly_category} onChange={c => setF({ ...f, firefly_category: c })} options={cats.map(c => ({ value: c, label: c }))} clearLabel="None" onCreate={c => c} /> : <input value={f.firefly_category} onChange={e => setF({ ...f, firefly_category: e.target.value })} placeholder="automobil" />}</Field>
          </div>
          <label className="row"><input type="checkbox" checked={f.firefly_auto_sync} onChange={e => setF({ ...f, firefly_auto_sync: e.target.checked })} style={{ width: 'auto' }} /> Sync tagged transactions automatically every 30 minutes</label>
          <div className="row"><button className="btn sm" onClick={() => run('firefly', f.firefly_url, f.firefly_token)} disabled={!f.firefly_url}>Test connection</button>{status('firefly')}</div>
        </div>
      </Card>
      <Card title="🛰 Dawarich" actions={badge(f.dawarich_configured)}>
        <p className="muted small">Kilometres you actually drove, per day, from the location history. Pocket Garage adds them on top of the last odometer reading, so the estimate (and reminders by distance) stay current between fill-ups. Turn it on per vehicle under the vehicle's Settings tab.</p>
        <div className="grid" style={{ gap: 12 }}>
          <Field label="URL"><input value={f.dawarich_url} onChange={e => setF({ ...f, dawarich_url: e.target.value })} placeholder="https://dawarich.example.com" /></Field>
          <Field label="API key" help="Dawarich → Account → API key."><input value={f.dawarich_api_key} onChange={e => setF({ ...f, dawarich_api_key: e.target.value })} autoComplete="off" /></Field>
          <div className="row"><button className="btn sm" onClick={() => run('dawarich', f.dawarich_url, f.dawarich_api_key)} disabled={!f.dawarich_url}>Test connection</button>{status('dawarich')}</div>
        </div>
      </Card>
      <Card title="🔔 Notifications" actions={badge(f.notify_configured)}>
        <p className="muted small">One push per vehicle when a reminder becomes <i>due soon</i>, <i>due</i> or <i>overdue</i>, plus expiring documents. Apprise gateway (<code>…/notify/&lt;key&gt;</code>), an ntfy topic URL, or any webhook that takes JSON.</p>
        <div className="grid" style={{ gap: 12 }}>
          <div className="two-col tight">
            <Field label="URL"><input value={f.notify_url} onChange={e => setF({ ...f, notify_url: e.target.value })} placeholder="http://apprise:8000/notify/garage" /></Field>
            <Field group label="Format"><Segmented value={f.notify_format} onChange={v => setF({ ...f, notify_format: v })} options={[{ value: 'apprise', label: 'Apprise' }, { value: 'ntfy', label: 'ntfy' }, { value: 'json', label: 'JSON' }]} size="sm" /></Field>
          </div>
          <div className="two-col tight">
            <Field label="Check at (hour, server time)"><input type="number" min={0} max={23} value={f.notify_hour} onChange={e => setF({ ...f, notify_hour: Number(e.target.value) })} /></Field>
            <Field group label="Overdue nag"><label className="row" style={{ paddingTop: 8 }}><input type="checkbox" checked={f.notify_digest} onChange={e => setF({ ...f, notify_digest: e.target.checked })} style={{ width: 'auto' }} /> repeat overdue items every Monday</label></Field>
          </div>
          <div className="row"><button className="btn sm" onClick={() => run('notify', f.notify_url, '', { format: f.notify_format })} disabled={!f.notify_url}>Send a test</button>{f.notify_configured && <button className="btn sm" onClick={notifyRun}>Push everything due now</button>}{status('notify')}</div>
        </div>
      </Card>
      <Card title="🧳 Pocket Travel" actions={viaBadge(f.travel_via)}>
        <p className="muted small">Fill-ups and costs that fall inside a trip show up on the vehicle overview per trip, with the kilometres driven. With Pocket Core on the box there is nothing to fill in: your own trips come through Core. A token from Pocket Travel (Settings → API tokens) entered here takes priority; clear it to go back to Core.</p>
        <div className="grid" style={{ gap: 12 }}>
          <div className="two-col tight">
            <Field label="URL"><input value={f.travel_url} onChange={e => setF({ ...f, travel_url: e.target.value })} placeholder="http://host.docker.internal:8174" /></Field>
            <Field label="API token"><input value={f.travel_token} onChange={e => setF({ ...f, travel_token: e.target.value })} autoComplete="off" /></Field>
          </div>
          <Field label="Public URL (optional)" help="What the browser opens for trip links when the URL above is internal (e.g. host.docker.internal)."><input value={f.travel_public_url} onChange={e => setF({ ...f, travel_public_url: e.target.value })} placeholder="https://travel.example.com" /></Field>
          <div className="row"><button className="btn sm" onClick={() => run('travel', f.travel_url, f.travel_token)} >Test connection</button>{status('travel')}</div>
        </div>
      </Card>
      <Card title="📦 Pocket Inventory" actions={viaBadge(f.inventory_via)}>
        <p className="muted small">Parts and fluids on the shelf. When you list parts on a record, the name field suggests your inventory items. With Pocket Core on the box there is nothing to fill in; a token from Pocket Inventory (Settings → API tokens) entered here takes priority, clear it to go back to Core.</p>
        <div className="grid" style={{ gap: 12 }}>
          <div className="two-col tight">
            <Field label="URL"><input value={f.inventory_url} onChange={e => setF({ ...f, inventory_url: e.target.value })} placeholder="http://host.docker.internal:8170" /></Field>
            <Field label="API token"><input value={f.inventory_token} onChange={e => setF({ ...f, inventory_token: e.target.value })} autoComplete="off" /></Field>
          </div>
          <div className="row"><button className="btn sm" onClick={() => run('inventory', f.inventory_url, f.inventory_token)} >Test connection</button>{status('inventory')}</div>
          <label className="row"><input type="checkbox" checked={f.inventory_take} onChange={e => setF({ ...f, inventory_take: e.target.checked })} style={{ width: 'auto' }} /> Take parts picked from Pocket Inventory off the shelf when the work is logged (edits adjust the difference, deleting the record puts them back)</label>
        </div>
      </Card>
      <div><button className="btn primary" onClick={() => save()} disabled={busy}>Save integrations</button></div>
    </div>
  )
}

function PocketMoneyCard({ f, setF, run, status }: { f: SettingsT; setF: (s: SettingsT) => void; run: (which: string, url: string, key: string) => void; status: (which: string) => React.ReactNode }) {
  const ms = useMoneyStatus()
  const [cats, setCats] = useState<{ id: number; full_name: string; name: string }[]>([])
  const acct = f.money_account || ms?.default_account || 0
  useEffect(() => { if (ms?.connected) api.get<typeof cats>(`${V1}/money/categories${acct ? `?account=${acct}` : ''}`).then(setCats).catch(() => setCats([])) }, [ms?.connected, acct])
  const badge = !ms ? null : ms.connected ? <span className="badge cat">{ms.via === 'core' ? 'connected through Pocket Core' : 'connected with a token'}</span> : <span className="badge">not connected</span>
  const catOpts = cats.map(c => ({ value: String(c.id), label: c.full_name || c.name }))
  return (
    <Card title="👛 Pocket Money" actions={badge}>
      <p className="muted small">Every fill-up, service, repair or expense you log can become an expense in your Pocket Money, with the vehicle's tag, a link back here and the cost in its own currency. Edits and deletes here follow it there. {ms?.core ? <>On this box it goes through <b>Pocket Core</b>: no token is needed, and you can switch the link off for yourself in Pocket Core (Settings → Apps acting for me).</> : <>Without Pocket Core, give it your Money URL and a token from Pocket Money → Settings → API tokens.</>}</p>
      {ms && !ms.connected && ms.error && <div className="callout mb small">{ms.error}</div>}
      <div className="grid" style={{ gap: 12 }}>
        <label className="row"><input type="checkbox" checked={f.money_enabled} onChange={e => setF({ ...f, money_enabled: e.target.checked })} style={{ width: 'auto' }} /> Offer “Create a transaction in Pocket Money” when adding a cost, ticked by default</label>
        {ms?.connected && <div className="three-col tight">
          <Field label="Book on" help="Each vehicle remembers the last account you picked on its forms."><Select value={String(f.money_account || '')} onChange={a => setF({ ...f, money_account: Number(a) || 0 })} options={ms.accounts.map(a => ({ value: String(a.id), label: a.name, hint: a.currency }))} placeholder={ms.accounts.find(a => a.id === ms.default_account)?.name || 'Choose…'} clearLabel="Everyday account" /></Field>
          <Field label="Category for fuel" help="Empty: a category named like gorivo / fuel."><Select value={String(f.money_category_fuel || '')} onChange={c => setF({ ...f, money_category_fuel: Number(c) || 0 })} options={catOpts} placeholder="By name" clearLabel="By name" /></Field>
          <Field label="Category for everything else" help="Empty: a category named like auto / car."><Select value={String(f.money_category || '')} onChange={c => setF({ ...f, money_category: Number(c) || 0 })} options={catOpts} placeholder="By name" clearLabel="By name" /></Field>
        </div>}
        <details>
          <summary className="small">{ms?.core ? 'Use a Money token instead of Pocket Core' : 'Money URL and token'}</summary>
          <div className="two-col tight mt-sm">
            <Field label="URL" help={ms?.core ? 'Empty: found through Pocket Core.' : 'Inside Docker use http://host.docker.internal:PORT.'}><input value={f.money_url} onChange={e => setF({ ...f, money_url: e.target.value })} placeholder="http://host.docker.internal:8178" /></Field>
            <Field label="API token" help="A token wins over Pocket Core. Clear it to go back."><input value={f.money_token} onChange={e => setF({ ...f, money_token: e.target.value })} autoComplete="off" placeholder="pm_…" /></Field>
          </div>
        </details>
        <div className="row"><button className="btn sm" onClick={() => run('money', f.money_url, f.money_token)}>Test connection</button>{status('money')}</div>
      </div>
    </Card>
  )
}

function Import() {
  const { toast } = useToast()
  const { settings, reloadVehicles } = useApp()
  const navigate = useNavigate()
  const [ll, setLL] = useState({ url: '', username: '', password: '', currency: settings?.home_currency || 'EUR' })
  const [list, setList] = useState<{ id: number; name: string; plate: string; imported: boolean }[] | null>(null)
  const [sel, setSel] = useState<number[]>([])
  const [busy, setBusy] = useState('')
  const [result, setResult] = useState<{ vehicles: { source: string; vehicle_id?: number; name?: string; counts?: Record<string, number>; skipped: number; error?: string; existing: boolean }[]; error?: string } | null>(null)
  const fetchList = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy('list')
    try { const l = await api.post<typeof list>(`${V1}/import/lubelogger/vehicles`, ll); setList(l); setSel((l || []).map(v => v.id)) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy('') }
  }
  const run = async () => { setBusy('import'); try { setResult(await api.post(`${V1}/import/lubelogger`, { ...ll, vehicles: sel })); reloadVehicles() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy('') } }
  const importJSON = async (file: File) => {
    setBusy('json')
    try { const v = await api.post<{ id: number; display_name: string }>(`${V1}/import/vehicle`, JSON.parse(await file.text())); toast(`Imported ${v.display_name}`); reloadVehicles(); navigate(`/vehicles/${v.id}`) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy('') }
  }
  return (
    <div className="two-col">
      <Card title="From LubeLogger">
        <p className="muted small">Vehicles with their photo, service / repair / upgrade / tax records, fuel, odometer readings, reminders, planner cards and notes. LubeLogger's mirrored “Auto Insert” odometer rows are skipped — here a record's odometer <i>is</i> the reading. Numbers are read in either locale, dates in any format. Re-running skips what is already here.</p>
        <form onSubmit={fetchList} className="grid" style={{ gap: 12 }}>
          <Field label="LubeLogger URL"><input value={ll.url} onChange={e => setLL({ ...ll, url: e.target.value })} placeholder="http://host.docker.internal:8162" required /></Field>
          <div className="three-col tight">
            <Field label="Username (if any)"><input value={ll.username} onChange={e => setLL({ ...ll, username: e.target.value })} autoComplete="off" /></Field>
            <Field label="Password"><input type="password" value={ll.password} onChange={e => setLL({ ...ll, password: e.target.value })} autoComplete="new-password" /></Field>
            <Field label="Costs are in" help="LubeLogger has no currency; the Pocket Homelab app's marker wins per row."><CurrencySelect value={ll.currency} onChange={c => setLL({ ...ll, currency: c })} /></Field>
          </div>
          <div><button className="btn" disabled={busy === 'list'}>{busy === 'list' ? 'Connecting…' : 'List vehicles'}</button></div>
        </form>
        {list && (
          <div className="mt">
            {list.length === 0 ? <p className="muted">No vehicles found.</p> : <>
              <table className="table compact"><tbody>{list.map(v => <tr key={v.id}><td style={{ width: 28 }}><input type="checkbox" checked={sel.includes(v.id)} onChange={e => setSel(s => e.target.checked ? [...s, v.id] : s.filter(x => x !== v.id))} style={{ width: 'auto' }} /></td><td><b>{v.name}</b>{v.imported && <span className="badge" style={{ marginLeft: 6 }}>already here — only new rows</span>}</td><td className="muted small mono">{v.plate}</td></tr>)}</tbody></table>
              <div className="mt-sm"><button className="btn primary" onClick={run} disabled={busy === 'import' || sel.length === 0}>{busy === 'import' ? 'Importing…' : `Import ${sel.length} vehicle${sel.length === 1 ? '' : 's'}`}</button></div>
            </>}
          </div>
        )}
        {result && <div className="callout success mt">{result.error && <div className="bad">✕ {result.error}</div>}{result.vehicles.map((r, i) => <div key={i}>{r.error ? <span className="bad">✕ {r.source}: {r.error}</span> : <span>✓ <a href={`/vehicles/${r.vehicle_id}`}>{r.name}</a> — {Object.entries(r.counts || {}).map(([k, v]) => `${v} ${k}`).join(', ') || 'nothing new'}{r.skipped > 0 && ` (${r.skipped} already here)`}</span>}</div>)}</div>}
      </Card>
      <Card title="From a JSON backup">
        <p className="muted small">A vehicle exported from Pocket Garage (vehicle Settings → JSON backup) imports as a new vehicle owned by you.</p>
        <label className="btn">{busy === 'json' ? 'Importing…' : 'Choose file…'}<input type="file" accept="application/json,.json" hidden onChange={e => e.target.files?.[0] && importJSON(e.target.files[0])} /></label>
      </Card>
    </div>
  )
}

function Users() {
  const { user, users, reloadMeta, status } = useApp()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<User> | null>(null)
  const [del, setDel] = useState<User | null>(null)
  const isAdmin = user?.role === 'admin'
  const run = async (fn: () => Promise<unknown>, msg: string) => { try { await fn(); await reloadMeta(); toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Card title="Users" actions={isAdmin && <button className="btn primary sm" onClick={() => setEdit({ role: 'member' })}>+ Add user</button>}>
      <p className="muted">Everyone signs in with their own account and keeps their own garage: a vehicle is visible only to its owner and the people the owner adds under the vehicle's Settings. Preferences and integrations (Firefly, Dawarich, notifications, Travel, Inventory) are personal too. Admins manage accounts and the box-wide settings but do not see anyone else's vehicles. The same email and password work as Basic auth for the phone app and Raycast.</p>
      <table className="table compact">
        <thead><tr><th /><th>Name</th><th>Email</th><th>Role</th><th /></tr></thead>
        <tbody>
          {users.map(u => (
            <tr key={u.id}>
              <td style={{ width: 32 }}><Avatar user={u} /></td>
              <td>{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge" style={{ marginLeft: 6 }}>you</span>}</td>
              <td className="muted">{u.email}</td>
              <td><span className={`badge ${u.role === 'admin' ? 'cat' : ''}`}>{u.role}</span></td>
              <td className="right">{isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm title={`Remove ${del.name || del.email}?`} message="Their account and the vehicles they own are deleted." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Card>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f); toast(user.id ? 'User saved' : 'User added'); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? 'Edit user' : 'Add user'} onClose={onClose}>
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add user'}</button></div>
      </form>
    </Modal>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', current_password: '', new_password: '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { setUser(await api.put(`${V1}/auth/me`, f)); setF(x => ({ ...x, current_password: '', new_password: '' })); toast('Profile saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Card title="Profile">
      <form onSubmit={save} className="grid" style={{ gap: 14, maxWidth: 480 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
        <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        {status?.auth_mode === 'local' && <>
          <hr />
          <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
          <Field label="New password" help="At least 8 characters. Leave empty to keep the current one."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
        </>}
        <div><button className="btn primary" disabled={busy}>Save</button></div>
      </form>
    </Card>
  )
}

function Tokens() {
  const { toast } = useToast()
  const { settings } = useApp()
  const { data, reload } = useAsync(() => api.get<APIToken[]>(`${V1}/tokens`), [])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')
  const [del, setDel] = useState<APIToken | null>(null)
  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const r = await api.post<{ token: string }>(`${V1}/tokens`, { name: name || 'token' }); setFresh(r.token); setName(''); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const base = settings?.base_url || window.location.origin
  return (
    <div className="two-col">
      <Card title="API tokens">
        <p className="muted">Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you: it can read and change your vehicles, feed a Homepage widget (<code>GET /api/v1/dashboard</code>) and authorise a calendar subscription (<code>/cal/&lt;vehicle&gt;.ics?token=…</code>). See <code>docs/api.md</code>.</p>
        <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. Homepage widget)" /><button className="btn primary">Create</button></form>
        {fresh && <div className="callout success mb"><b>Copy it now — it is shown once.</b><pre className="mono" style={{ margin: '8px 0 0' }}>{fresh}</pre></div>}
        {data && data.length > 0 ? (
          <table className="table compact">
            <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
            <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? fmtDateTime(t.last_used_at) : 'never'}</td><td className="muted">{fmtDateTime(t.created_at)}</td><td className="right"><button className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
          </table>
        ) : <p className="muted">No tokens yet.</p>}
        {del && <Confirm title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
      </Card>
      <Card title="Phone app & Raycast (LubeLogger-compatible API)">
        <p className="muted">Pocket Garage answers LubeLogger's API at <code>{base}/api/…</code>: <code>/api/vehicles</code>, <code>/api/vehicle/info</code>, <code>/api/vehicle/gasrecords</code>, <code>…/servicerecords</code>, <code>…/repairrecords</code>, <code>…/upgraderecords</code>, <code>…/taxrecords</code>, <code>…/odometerrecords</code>, <code>…/reminders</code>, each with <code>/add</code>, <code>/update</code> and <code>/delete</code>, plus <code>/api/vehicles/add</code>.</p>
        <p className="muted">Point the <b>Pocket Homelab</b> app's LubeLogger URL and the <b>Raycast</b> extension at <code>{base}</code> and sign in with your Pocket Garage <b>email and password</b> (Basic auth) — or a bearer token from the left. Numbers are accepted with a comma or a dot; dates come back as ISO. Tax records map to expenses; fuel keeps the app's currency marker.</p>
      </Card>
    </div>
  )
}

function Data() {
  const { user } = useApp()
  return (
    <div className="two-col">
      {user?.role === 'admin' && <Card title="Backup (admin)">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>) with every vehicle. Uploaded documents and photos live next to it under <code>files/</code>. Restore by stopping the app and replacing <code>pocket-garage.db</code> in the data directory.</p>
        <a className="btn primary" href={`${V1}/backup`}>Download database</a>
      </Card>}
      <Card title="Per-vehicle exports">
        <p className="muted">Each vehicle's Settings tab offers a JSON backup (imports again under Import), CSV for records, fuel and odometer, and a calendar (.ics) of its reminders.</p>
      </Card>
    </div>
  )
}

function About() {
  const { data } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const counts = (data?.counts || {}) as Record<string, number>
  return (
    <Card title="About">
      <p><b>Pocket Garage</b> keeps every vehicle on one page: fuel with real economy, service and repairs with the parts fitted, expenses, an odometer that estimates itself from Dawarich, reminders that roll forward, a planner board, notes and documents — with the real spending from Firefly III and a LubeLogger-compatible API so the phone app and Raycast keep working. One binary, one SQLite file, one volume.</p>
      <p className="muted small">Open source (MIT) — <a href="https://github.com/abjelosevic88/pocket-homelab-garage" target="_blank" rel="noreferrer">github.com/abjelosevic88/pocket-homelab-garage</a>. Sibling projects: Pocket Habits, Pocket Inventory, Pocket Bookshelf, Pocket Split, Pocket Clips, Pocket Subtitles, Pocket Invoicing and Pocket Travel.</p>
      {data && (
        <table className="table compact mt">
          <tbody>
            <tr><th>Version</th><td className="mono">{String(data.version)}</td></tr>
            <tr><th>Runtime</th><td className="mono">{String(data.go)} · {String(data.os)}/{String(data.arch)}</td></tr>
            {data.auth_mode !== undefined && <tr><th>Auth mode</th><td className="mono">{String(data.auth_mode)}</td></tr>}
            {data.db_path !== undefined && <tr><th>Database</th><td className="mono">{String(data.db_path)} · {fmtBytes(Number(data.db_bytes))} · files {fmtBytes(Number(data.files_bytes))}</td></tr>}
            <tr><th>Timezone</th><td className="mono">{String(data.timezone)}</td></tr>
            <tr><th>Uptime</th><td className="mono">{String(data.uptime)}</td></tr>
            {data.counts !== undefined && <tr><th>Rows</th><td className="mono">{counts.vehicles} vehicles · {counts.fuel} fill-ups · {counts.records} records · {counts.odometer} readings · {counts.reminders} reminders · {counts.documents} documents · {counts.users} users</td></tr>}
          </tbody>
        </table>
      )}
    </Card>
  )
}
