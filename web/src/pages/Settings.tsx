import React, { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, ConnMode, Settings as SettingsT, User } from '../lib/types'
import { DATE_FORMATS, TIME_FORMATS, fmtBytes, fmtDateTime, type DateFormat, type TimeFormat } from '../lib/format'
import { ECON_OPTIONS, VOL_OPTIONS } from '../lib/units'
import { Avatar, Confirm, Field, Loading, Modal, Segmented, SettingRow, Switch, Tabs, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { CurrencySelect } from '../components/Money'
import { useMoneyStatus } from '../components/MoneyBook'

type TabId = 'general' | 'notifications' | 'apps' | 'import' | 'profile' | 'people' | 'tokens' | 'data' | 'about'

const TABS: { id: TabId; label: string; icon: string; sub: string }[] = [
  { id: 'general', label: 'General', icon: 'settings', sub: 'Currency, units, date and time formats, and the app title.' },
  { id: 'notifications', label: 'Notifications', icon: 'bell', sub: 'Where pushes for due reminders and expiring documents are sent.' },
  { id: 'apps', label: 'Connected apps', icon: 'layers', sub: 'Pocket Money, Dawarich, Pocket Travel and Pocket Inventory — through Pocket Core or directly.' },
  { id: 'import', label: 'Import', icon: 'download', sub: 'Bring vehicles over from LubeLogger, or restore a vehicle from a JSON backup.' },
  { id: 'profile', label: 'Profile', icon: 'user', sub: 'Your name, email and password.' },
  { id: 'people', label: 'People', icon: 'users', sub: 'Everyone who has an account on this box.' },
  { id: 'tokens', label: 'API & phone', icon: 'key', sub: 'Personal tokens, and the LubeLogger-compatible API for the phone app and Raycast.' },
  { id: 'data', label: 'Backup & data', icon: 'database', sub: 'Download the database and export single vehicles.' },
  { id: 'about', label: 'About', icon: 'info', sub: 'Version, numbers and the server.' },
]
// the old ?tab= names
const ALIAS: Record<string, TabId> = { integrations: 'apps', users: 'people', api: 'tokens' }

export default function Settings() {
  const { tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const legacy = new URLSearchParams(window.location.search).get('tab') || ''
  const want = raw || ALIAS[legacy] || legacy
  const tab: TabId = (TABS.find(t => t.id === (ALIAS[want] || want))?.id) || 'general'
  const current = TABS.find(t => t.id === tab)!
  const go = (id: TabId) => navigate(`/settings/${id}`, { replace: true })

  useEffect(() => { document.querySelector('.set-content')?.scrollTo({ top: 0 }) }, [tab])
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-backdrop, .menu, .sel-pop')) navigate('/') }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [navigate])

  return (
    <div className="set-page">
      <header className="set-header">
        <button type="button" className="btn ghost icon-only" aria-label="Back to the garage" title="Back to the garage (esc)" onClick={() => navigate('/')}><Icon name="chevron-left" size={18} /></button>
        <img src="/favicon.svg" alt="" width={24} height={24} className="set-logo" />
        <h1>Settings</h1>
      </header>
      {mobile && <div className="set-tabs-mobile"><Tabs tabs={TABS.map(t => ({ id: t.id, label: t.label, icon: t.icon }))} value={tab} onChange={go} /></div>}
      <div className="set-body">
        {!mobile && (
          <nav className="set-nav" aria-label="Settings sections">
            {TABS.map(t => (
              <button key={t.id} type="button" className={`set-nav-item ${t.id === tab ? 'active' : ''}`} aria-current={t.id === tab ? 'page' : undefined} onClick={() => go(t.id)}>
                <Icon name={t.icon} size={16} />{t.label}
              </button>
            ))}
          </nav>
        )}
        <main className="set-content">
          <div className="set-inner" key={tab}>
            <div className="set-title">
              <h2>{current.label}</h2>
              <p>{current.sub}</p>
            </div>
            {tab === 'general' && <General />}
            {tab === 'notifications' && <Notifications />}
            {tab === 'apps' && <Integrations />}
            {tab === 'import' && <Import />}
            {tab === 'profile' && <Profile />}
            {tab === 'people' && <Users />}
            {tab === 'tokens' && <Tokens />}
            {tab === 'data' && <Data />}
            {tab === 'about' && <About />}
          </div>
        </main>
      </div>
    </div>
  )
}

/** A settings card: title, explanation, actions on the right — Pocket Calendar's section. */
function Section({ title, sub, icon, actions, children }: { title: React.ReactNode; sub?: React.ReactNode; icon?: string; actions?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <section className="set-section">
      <div className="set-section-head">
        {icon && <span className="set-app-ico"><Icon name={icon} size={17} /></span>}
        <div className="grow"><h3>{title}</h3>{sub && <p>{sub}</p>}</div>
        {actions && <div className="row">{actions}</div>}
      </div>
      {children && <div className="set-section-body">{children}</div>}
    </section>
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
    <div className="stack loose">
      <Section title="Money & units" sub="Yours alone — everyone on the box picks their own.">
        <SettingRow title="Home currency" help="Totals across vehicles convert into this. Each vehicle has its own currency too."><CurrencySelect value={f.home_currency} onChange={c => setF({ ...f, home_currency: c })} /></SettingRow>
        <SettingRow title="New vehicles use"><Segmented value={f.distance_unit} onChange={v => setF({ ...f, distance_unit: v as 'km' | 'mi' })} options={[{ value: 'km', label: 'km' }, { value: 'mi', label: 'miles' }]} size="sm" /></SettingRow>
        <SettingRow title="Volume"><Select value={f.volume_unit} onChange={v => setF({ ...f, volume_unit: v as SettingsT['volume_unit'] })} options={VOL_OPTIONS.map(o => ({ value: o.id, label: o.label }))} /></SettingRow>
        <SettingRow title="Economy"><Select value={f.economy_unit} onChange={v => setF({ ...f, economy_unit: v as SettingsT['economy_unit'] })} options={ECON_OPTIONS.map(o => ({ value: o.id, label: o.label }))} /></SettingRow>
      </Section>
      <Section title="Date & time">
        <SettingRow title="Date format"><Select value={f.date_format} onChange={v => setF({ ...f, date_format: v as DateFormat })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        <SettingRow title="Time format"><Select value={f.time_format} onChange={v => setF({ ...f, time_format: v as TimeFormat })} options={TIME_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
      </Section>
      <Section title="This box" sub={isAdmin ? 'These apply to everyone on this box.' : 'Set by an admin for everyone.'}>
        <SettingRow title="App title" help="Shown in the sidebar and the browser tab."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} maxLength={40} disabled={!isAdmin} style={{ width: 220 }} /></SettingRow>
        <SettingRow title="Look up exchange rates" help={f.fx_provider === 'none' ? 'Disabled on the server.' : 'Frankfurter / open.er-api.com; euro pegs like BAM are built in.'}>
          <Switch checked={f.fx_auto} onChange={v => setF({ ...f, fx_auto: v })} disabled={!isAdmin || f.fx_provider === 'none'} label="Look up exchange rates" />
        </SettingRow>
      </Section>
      <div className="row"><button className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
      <Section title="How it fits together">
        <dl className="set-dl">
          <div><dt>Fuel</dt><dd className="muted">One row per fill-up. Distance and economy come from the previous full tank; partial fills roll into the next full one.</dd></div>
          <div><dt>Service & costs</dt><dd className="muted">Service, repairs, upgrades and plain expenses in one list with a kind, a category and the parts fitted.</dd></div>
          <div><dt>Odometer</dt><dd className="muted">Every fill-up or record with a reading <i>is</i> the odometer log. Dawarich adds the kilometres driven since.</dd></div>
          <div><dt>Reminders</dt><dd className="muted">By date, by odometer or both. Recurring ones roll forward when done; logging matching work completes them.</dd></div>
          <div><dt>Pocket Money</dt><dd className="muted">Costs you log can become transactions in your Pocket Money (through Pocket Core, or a token). Edits and deletes follow them.</dd></div>
          <div><dt>Phone & Raycast</dt><dd className="muted">The LubeLogger-shaped API lives at <code>/api/…</code>, so the Pocket Homelab app and the Raycast extension only need the new URL.</dd></div>
        </dl>
      </Section>
    </div>
  )
}

function Notifications() {
  const { f, setF, save, busy } = useSettingsForm()
  const { toast } = useToast()
  const [test, setTest] = useState('')
  if (!f) return <Loading />
  const runTest = async () => {
    setTest('testing…')
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which: 'notify', url: f.notify_url, key: '', format: f.notify_format }); setTest(r.ok ? `✓ ${r.message}` : `✕ ${r.error}`) } catch (e) { setTest(`✕ ${(e as Error).message}`) }
  }
  const notifyRun = async () => { try { const r = await api.post<{ sent: number }>(`${V1}/notify/run?force=1`); toast(`${r.sent} message${r.sent === 1 ? '' : 's'} sent`) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <div className="stack loose">
      <Section title="Push notifications" icon="bell" actions={<span className={`badge ${f.notify_configured ? 'ok' : ''}`}>{f.notify_configured ? 'on' : 'not set'}</span>}
        sub={<>One push per vehicle when a reminder becomes <i>due soon</i>, <i>due</i> or <i>overdue</i>, plus expiring documents. Apprise gateway (<code>…/notify/&lt;key&gt;</code>), an ntfy topic URL, or any webhook that takes JSON.</>}>
        <SettingRow title="URL" stack><input value={f.notify_url} onChange={e => setF({ ...f, notify_url: e.target.value })} placeholder="http://apprise:8000/notify/garage" /></SettingRow>
        <SettingRow title="Format"><Segmented value={f.notify_format} onChange={v => setF({ ...f, notify_format: v })} options={[{ value: 'apprise', label: 'Apprise' }, { value: 'ntfy', label: 'ntfy' }, { value: 'json', label: 'JSON' }]} size="sm" /></SettingRow>
        <SettingRow title="Check at" help="Hour of the day, server time."><input type="number" min={0} max={23} value={f.notify_hour} onChange={e => setF({ ...f, notify_hour: Number(e.target.value) })} className="set-time" /></SettingRow>
        <SettingRow title="Overdue nag" help="Repeat overdue items every Monday."><Switch checked={f.notify_digest} onChange={v => setF({ ...f, notify_digest: v })} label="Overdue nag" /></SettingRow>
      </Section>
      <div className="row wrap">
        <button className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        <button className="btn" onClick={runTest} disabled={!f.notify_url}><Icon name="mail" size={15} />Send a test</button>
        {f.notify_configured && <button className="btn" onClick={notifyRun}><Icon name="bell" size={15} />Push everything due now</button>}
        {test && <span className={`small ${test.startsWith('✓') ? 'ok' : test.startsWith('✕') ? 'bad' : 'muted'}`}>{test}</span>}
      </div>
    </div>
  )
}

function Integrations() {
  const { f, setF, save, busy } = useSettingsForm()
  const [test, setTest] = useState<Record<string, string>>({})
  if (!f) return <Loading />
  const run = async (which: string, url: string, key: string, extra: Record<string, string> = {}) => {
    setTest(t => ({ ...t, [which]: 'testing…' }))
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which, url, key, ...extra }); setTest(t => ({ ...t, [which]: r.ok ? `✓ ${r.message}` : `✕ ${r.error}` })) } catch (e) { setTest(t => ({ ...t, [which]: `✕ ${(e as Error).message}` })) }
  }
  const status = (which: string) => <span className={`small ${test[which]?.startsWith('✓') ? 'ok' : test[which]?.startsWith('✕') ? 'bad' : 'muted'}`}>{test[which]}</span>
  return (
    <div className="stack loose">
      <PocketMoneyCard f={f} setF={setF} run={run} status={status} save={save} />
      <Section title="Dawarich" icon="route" actions={connBadge(f.dawarich_mode, f.dawarich_via)}
        sub="Kilometres you actually drove, per day, from the location history. Pocket Garage adds them on top of the last odometer reading, so the estimate (and reminders by distance) stay current between fill-ups. Turn it on per vehicle under the vehicle's Settings tab.">
        <Connection app="Dawarich" service source={f.dawarich_source} mode={f.dawarich_mode} via={f.dawarich_via} detail={f.dawarich_detail} core={f.core_configured} hasToken={!!f.dawarich_api_key}
          onMode={m => save({ dawarich_mode: m })} onDisconnect={() => save({ dawarich_url: '', dawarich_api_key: '' })}
          test={<><button className="btn sm" onClick={() => run('dawarich', f.dawarich_url, f.dawarich_api_key)}>Test connection</button>{status('dawarich')}</>}
          direct={<div className="two-col tight">
            <Field label="URL"><input value={f.dawarich_url} onChange={e => setF({ ...f, dawarich_url: e.target.value })} placeholder="https://dawarich.example.com" /></Field>
            <Field label="API key" help="Dawarich → Account → API key."><input value={f.dawarich_api_key} onChange={e => setF({ ...f, dawarich_api_key: e.target.value })} autoComplete="off" /></Field>
          </div>} />
      </Section>
      <Section title="Pocket Travel" icon="plane" actions={connBadge(f.travel_mode, f.travel_via)} sub="Fill-ups and costs that fall inside a trip show up on the vehicle overview per trip, with the kilometres driven.">
        <Connection app="Pocket Travel" mode={f.travel_mode} via={f.travel_via} detail={f.travel_detail} core={f.core_configured} hasToken={!!f.travel_token}
          onMode={m => save({ travel_mode: m })} onDisconnect={() => save({ travel_url: '', travel_token: '', travel_public_url: '' })}
          test={<><button className="btn sm" onClick={() => run('travel', f.travel_url, f.travel_token)}>Test connection</button>{status('travel')}</>}
          direct={<>
            <div className="two-col tight">
              <Field label="URL"><input value={f.travel_url} onChange={e => setF({ ...f, travel_url: e.target.value })} placeholder="http://host.docker.internal:8174" /></Field>
              <Field label="API token" help="Pocket Travel → Settings → API tokens"><input value={f.travel_token} onChange={e => setF({ ...f, travel_token: e.target.value })} autoComplete="off" /></Field>
            </div>
            <Field label="Public URL (optional)" help="What the browser opens for trip links when the URL above is internal (e.g. host.docker.internal)."><input value={f.travel_public_url} onChange={e => setF({ ...f, travel_public_url: e.target.value })} placeholder="https://travel.example.com" /></Field>
          </>} />
      </Section>
      <Section title="Pocket Inventory" icon="package" actions={connBadge(f.inventory_mode, f.inventory_via)} sub="Parts and fluids on the shelf. When you list parts on a record, the name field suggests your inventory items.">
        <Connection app="Pocket Inventory" mode={f.inventory_mode} via={f.inventory_via} detail={f.inventory_detail} core={f.core_configured} hasToken={!!f.inventory_token}
          onMode={m => save({ inventory_mode: m })} onDisconnect={() => save({ inventory_url: '', inventory_token: '' })}
          test={<><button className="btn sm" onClick={() => run('inventory', f.inventory_url, f.inventory_token)}>Test connection</button>{status('inventory')}</>}
          direct={<div className="two-col tight">
            <Field label="URL"><input value={f.inventory_url} onChange={e => setF({ ...f, inventory_url: e.target.value })} placeholder="http://host.docker.internal:8170" /></Field>
            <Field label="API token" help="Pocket Inventory → Settings → API tokens"><input value={f.inventory_token} onChange={e => setF({ ...f, inventory_token: e.target.value })} autoComplete="off" /></Field>
          </div>} />
        {f.inventory_mode !== 'off' && <div className="mt"><SettingRow title="Take parts off the shelf" help={<>Parts picked from Pocket Inventory come off the shelf when the work is logged (edits adjust the difference, deleting the record puts them back)</>}><Switch checked={f.inventory_take} onChange={v => setF({ ...f, inventory_take: v })} label="Take parts off the shelf" /></SettingRow></div>}
      </Section>
      <div className="row"><button className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

/**
 * One connection — a Pocket app, or a third-party service (`service`, e.g.
 * Dawarich) whose address and key come from Pocket Core — as Pocket Core,
 * Direct (URL + token / API key) or Off, with what it actually is right now.
 * The mode saves at once; Disconnect removes the saved URL and token.
 * With Pocket Core on the box the switch is Pocket Core | Off and Direct is
 * an advanced option behind a link (always shown while it is the mode in
 * use); without Core, Direct is the only way in and shows as usual.
 */
function Connection({ app, mode, via, detail, core, onMode, onDisconnect, hasToken, test, direct, service, source }: {
  app: string; mode: ConnMode; via: string; detail: string; core: boolean; onMode: (m: ConnMode) => void; onDisconnect: () => void
  hasToken: boolean; test: React.ReactNode; direct: React.ReactNode; service?: boolean; source?: string
}) {
  const ok = via !== ''
  const [advanced, setAdvanced] = useState(false)
  const showDirect = !core || mode === 'direct' || advanced
  const secret = service ? 'API key' : 'token'
  const coreOk = service
    ? `Connected through Pocket Core (${source === 'box' ? "the box's key" : 'your own key'}). Nothing to fill in; switch it off for yourself in Pocket Core → Settings → Apps acting for me.`
    : `Connected through Pocket Core, as you. Nothing to fill in; switch it off for yourself in Pocket Core → Settings → Apps acting for me.`
  const line = mode === 'off' ? `Off: Garage does not talk to ${app}.`
    : mode === 'core' ? (ok ? coreOk : `Not connected: ${detail}.`)
    : (ok ? `Direct connection with your own ${app} URL and ${secret}; use Test connection to check it.` : `Not connected: ${detail || `add the URL and ${service ? 'an API key' : 'a token'}`}.`)
  return (
    <div className="conn-block">
      <div className="conn-mode">
        <Segmented size="sm" value={mode} onChange={m => onMode(m as ConnMode)} options={[
          { value: 'core', label: 'Pocket Core', title: core ? 'Through Pocket Core: per person, nothing to paste' : 'There is no Pocket Core on this box' },
          ...(showDirect ? [{ value: 'direct', label: 'Direct', title: service ? 'Your own URL and API key' : 'Your own URL and API token' }] : []),
          { value: 'off', label: 'Off' },
        ]} />
        {!showDirect && <button type="button" className="conn-advanced" onClick={() => setAdvanced(true)}>Advanced: connect directly with a URL and key</button>}
      </div>
      <div className={`conn-status ${mode === 'off' ? '' : ok ? 'ok' : 'bad'}`}><span className="dot" />{line}</div>
      {mode === 'direct' && <>
        {direct}
        <div className="row wrap">{test}{hasToken && <button className="btn sm ghost danger-text" onClick={onDisconnect} title={`Remove the saved URL and ${secret}`}>Disconnect</button>}</div>
      </>}
      {mode === 'core' && <div className="row wrap">{test}</div>}
    </div>
  )
}

export const connBadge = (mode: ConnMode, via: string) => mode === 'off' ? <span className="badge">off</span>
  : via ? <span className="badge ok">{via === 'core' ? 'Pocket Core' : 'direct'}</span>
  : <span className="badge warn">not connected</span>

function PocketMoneyCard({ f, setF, run, status, save }: { f: SettingsT; setF: (s: SettingsT) => void; run: (which: string, url: string, key: string) => void; status: (which: string) => React.ReactNode; save: (patch?: Partial<SettingsT>) => Promise<void> }) {
  const ms = useMoneyStatus(undefined, `${f.money_mode}:${f.money_via}`)
  const [cats, setCats] = useState<{ id: number; full_name: string; name: string }[]>([])
  const acct = f.money_account || ms?.default_account || 0
  useEffect(() => { if (ms?.connected) api.get<typeof cats>(`${V1}/money/categories${acct ? `?account=${acct}` : ''}`).then(setCats).catch(() => setCats([])) }, [ms?.connected, acct])
  const catOpts = cats.map(c => ({ value: String(c.id), label: c.full_name || c.name }))
  return (
    <Section title="Pocket Money" icon="wallet" actions={connBadge(f.money_mode, f.money_via)} sub="Every fill-up, service, repair or expense you log can become an expense in your Pocket Money, with the vehicle's tag, a link back here and the cost in its own currency. Edits and deletes here follow it there.">
      <Connection app="Pocket Money" mode={f.money_mode} via={f.money_via} detail={f.money_detail} core={f.core_configured} hasToken={!!f.money_token}
        onMode={m => save({ money_mode: m })} onDisconnect={() => save({ money_url: '', money_token: '' })}
        test={<><button className="btn sm" onClick={() => run('money', f.money_url, f.money_token)}>Test connection</button>{status('money')}</>}
        direct={<div className="two-col tight">
          <Field label="URL" help="Inside Docker use http://host.docker.internal:PORT."><input value={f.money_url} onChange={e => setF({ ...f, money_url: e.target.value })} placeholder="http://host.docker.internal:8178" /></Field>
          <Field label="API token" help="Pocket Money → Settings → API tokens"><input value={f.money_token} onChange={e => setF({ ...f, money_token: e.target.value })} autoComplete="off" placeholder="pm_…" /></Field>
        </div>} />
      {f.money_mode !== 'off' && <div className="grid mt" style={{ gap: 12 }}>
        <SettingRow title="Offer “Create a transaction in Pocket Money”" help={<>When adding a cost, ticked by default</>}><Switch checked={f.money_enabled} onChange={v => setF({ ...f, money_enabled: v })} label="Offer Pocket Money" /></SettingRow>
        {ms?.connected && <div className="three-col tight">
          <Field label="Book on" help="Each vehicle remembers the last account you picked on its forms."><Select value={String(f.money_account || '')} onChange={a => setF({ ...f, money_account: Number(a) || 0 })} options={ms.accounts.map(a => ({ value: String(a.id), label: a.name, hint: a.currency }))} placeholder={ms.accounts.find(a => a.id === ms.default_account)?.name || 'Choose…'} clearLabel="Everyday account" /></Field>
          <Field label="Category for fuel" help="Empty: a category named like gorivo / fuel."><Select value={String(f.money_category_fuel || '')} onChange={c => setF({ ...f, money_category_fuel: Number(c) || 0 })} options={catOpts} placeholder="By name" clearLabel="By name" /></Field>
          <Field label="Category for everything else" help="Empty: a category named like auto / car."><Select value={String(f.money_category || '')} onChange={c => setF({ ...f, money_category: Number(c) || 0 })} options={catOpts} placeholder="By name" clearLabel="By name" /></Field>
        </div>}
      </div>}
    </Section>
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
    <div className="stack loose">
      <Section title="From LubeLogger" icon="download" sub={<>Vehicles with their photo, service / repair / upgrade / tax records, fuel, odometer readings, reminders, planner cards and notes. LubeLogger's mirrored “Auto Insert” odometer rows are skipped — here a record's odometer <i>is</i> the reading. Numbers are read in either locale, dates in any format. Re-running skips what is already here.</>}>
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
      </Section>
      <Section title="From a JSON backup" icon="upload" sub="A vehicle exported from Pocket Garage (vehicle Settings → JSON backup) imports as a new vehicle owned by you.">
        <label className="btn"><Icon name="upload" size={15} />{busy === 'json' ? 'Importing…' : 'Choose file…'}<input type="file" accept="application/json,.json" hidden onChange={e => e.target.files?.[0] && importJSON(e.target.files[0])} /></label>
      </Section>
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
    <Section title="People on this box" actions={isAdmin && <button className="btn primary sm" onClick={() => setEdit({ role: 'member' })}><Icon name="plus" size={14} />Add person</button>} sub={<>Everyone signs in with their own account and keeps their own garage: a vehicle is visible only to its owner and the people the owner adds under the vehicle's Settings. Preferences and integrations (Pocket Money, Dawarich, notifications, Travel, Inventory) are personal too. Admins manage accounts and the box-wide settings but do not see anyone else's vehicles. The same email and password work as Basic auth for the phone app and Raycast.</>}>
      <div className="set-people">
        {users.map(u => (
          <div key={u.id} className="set-person">
            <Avatar name={u.name} email={u.email} size={34} />
            <div className="grow">
              <div className="set-person-name">{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge">you</span>}{u.role === 'admin' && <span className="badge accent">admin</span>}</div>
              <div className="muted small ellipsis">{u.email}</div>
            </div>
            {isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}
          </div>
        ))}
      </div>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm confirmLabel="Remove" title={`Remove ${del.name || del.email}?`} message="Their account and the vehicles they own are deleted." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Section>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f); toast(user.id ? 'Saved' : 'Person added'); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? 'Edit person' : 'Add a person'} onClose={onClose} size="sm">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add person'}</button></div>
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
    <form onSubmit={save} className="stack loose">
      <div className="set-profile">
        <Avatar name={user?.name} email={user?.email} size={56} />
        <div className="grow"><h3>{user?.name || user?.email}</h3><p className="muted small">{user?.email} · {user?.role}</p></div>
      </div>
      <Section title="You">
        <div className="stack">
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
        <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        </div>
      </Section>
        {status?.auth_mode === 'local' && <Section title="Password" sub="Leave empty to keep the current one."><div className="stack">
          <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
          <Field label="New password" help="At least 8 characters."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
        </div></Section>}
        <div className="row"><button className="btn primary" disabled={busy}>Save changes</button></div>
    </form>
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
    <div className="stack loose">
      <Section title="Personal tokens" icon="key" sub={<>Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you: it can read and change your vehicles, feed a Homepage widget (<code>GET /api/v1/dashboard</code>) and authorise a calendar subscription (<code>/cal/&lt;vehicle&gt;.ics?token=…</code>). See <code>docs/api.md</code>.</>}>
        <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. Homepage widget)" /><button className="btn primary"><Icon name="plus" size={15} />Create</button></form>
        {fresh && <div className="set-fresh mb"><div className="row"><Icon name="check-circle" size={16} /><b>Copy it now — it is shown once.</b></div><pre className="mono">{fresh}</pre></div>}
        {data && data.length > 0 ? (
          <table className="table compact">
            <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
            <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? fmtDateTime(t.last_used_at) : 'never'}</td><td className="muted">{fmtDateTime(t.created_at)}</td><td className="right"><button className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
          </table>
        ) : <p className="muted">No tokens yet.</p>}
        {del && <Confirm confirmLabel="Revoke" title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
      </Section>
      <Section title="Phone app & Raycast" icon="smartphone" sub="The LubeLogger-compatible API.">
        <p className="muted">Pocket Garage answers LubeLogger's API at <code>{base}/api/…</code>: <code>/api/vehicles</code>, <code>/api/vehicle/info</code>, <code>/api/vehicle/gasrecords</code>, <code>…/servicerecords</code>, <code>…/repairrecords</code>, <code>…/upgraderecords</code>, <code>…/taxrecords</code>, <code>…/odometerrecords</code>, <code>…/reminders</code>, each with <code>/add</code>, <code>/update</code> and <code>/delete</code>, plus <code>/api/vehicles/add</code>.</p>
        <p className="muted">Point the <b>Pocket Homelab</b> app's LubeLogger URL and the <b>Raycast</b> extension at <code>{base}</code> and sign in with your Pocket Garage <b>email and password</b> (Basic auth) — or a bearer token from the left. Numbers are accepted with a comma or a dot; dates come back as ISO. Tax records map to expenses; fuel keeps the app's currency marker.</p>
      </Section>
    </div>
  )
}

function Data() {
  const { user } = useApp()
  return (
    <div className="stack loose">
      {user?.role === 'admin' && <Section title="Database backup" icon="database" sub="Admins only." >
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>) with every vehicle. Uploaded documents and photos live next to it under <code>files/</code>. Restore by stopping the app and replacing <code>pocket-garage.db</code> in the data directory.</p>
        <a className="btn primary" href={`${V1}/backup`}><Icon name="download" size={15} />Download database</a>
      </Section>}
      <Section title="Per-vehicle exports" icon="car" sub="Each vehicle's Settings tab offers a JSON backup (imports again under Import), CSV for records, fuel and odometer, and a calendar (.ics) of its reminders." />
    </div>
  )
}

function About() {
  const { data, error } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return <Loading />
  const counts = (data.counts || {}) as Record<string, number>
  const tiles: [string, number | undefined, string][] = [['Vehicles', counts.vehicles, 'car'], ['Fill-ups', counts.fuel, 'fuel'], ['Records', counts.records, 'wrench'], ['Reminders', counts.reminders, 'bell']]
  const rows: [string, React.ReactNode][] = [
    ['Version', <code key="v">{String(data.version)}</code>],
    ['Uptime', String(data.uptime)],
    ['Runtime', `${String(data.go)} · ${String(data.os)}/${String(data.arch)}`],
    ['Time zone', String(data.timezone)],
  ]
  if (data.auth_mode !== undefined) rows.push(['Sign-in', String(data.auth_mode)])
  if (data.db_path !== undefined) rows.push(['Database', <span key="db"><code>{String(data.db_path)}</code> · {fmtBytes(Number(data.db_bytes))} · files {fmtBytes(Number(data.files_bytes))}</span>])
  if (data.counts !== undefined) rows.push(['More rows', `${counts.odometer ?? 0} readings · ${counts.documents ?? 0} documents · ${counts.plans ?? 0} plans · ${counts.notes ?? 0} notes`])
  return (
    <div className="stack loose">
      <div className="set-about-hero">
        <img src="/favicon.svg" alt="" width={48} height={48} />
        <div>
          <h3>Pocket Garage</h3>
          <p className="muted small">Every vehicle on one page: fuel with real economy, service and repairs with the parts fitted, expenses, a self-estimating odometer, reminders, a planner, notes and documents — part of the Pocket apps.</p>
        </div>
      </div>
      <div className="set-stats">
        {tiles.map(([label, n, icon]) => (
          <div key={label} className="set-stat"><Icon name={icon} size={16} /><span className="set-stat-n tnum">{n ?? 0}</span><span className="set-stat-l">{label}</span></div>
        ))}
      </div>
      <Section title="This server">
        <dl className="set-dl">
          {rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
        </dl>
      </Section>
      <div className="row wrap">
        <button className="btn" onClick={() => window.dispatchEvent(new CustomEvent('pg:shortcuts'))}><Icon name="keyboard" size={15} />Keyboard shortcuts</button>
        <a className="btn" href="https://github.com/abjelosevic88/pocket-homelab-garage" target="_blank" rel="noreferrer"><Icon name="external" size={15} />Project on GitHub</a>
      </div>
    </div>
  )
}
