// Left column: log button, sections, the vehicles, settings — the Pocket Calendar sidebar.
import { useEffect, useState } from 'react'
import { Link, NavLink, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Vehicle } from '../lib/types'
import { Icon } from './icons'
import { VehicleIcon } from './vehicle'
import type { LogKind } from './Layout'
import { type Anchor, type MenuItem, Menu, rectOf } from './ui'

export function Sidebar({ onLog, onNavigate, target }: { onLog: (k: LogKind, v?: Vehicle) => void; onNavigate: () => void; target?: Vehicle }) {
  const { vehicles, status, settings } = useApp()
  const [menu, setMenu] = useState<Anchor | null>(null)
  // summaries for the due counts (the context list is bare)
  const [due, setDue] = useState<Record<number, number>>({})
  useEffect(() => {
    let live = true
    api.get<Vehicle[]>(`${V1}/vehicles?status=active`).then(list => { if (live) setDue(Object.fromEntries((list || []).map(v => [v.id, v.summary?.reminders_due || 0]))) }).catch(() => {})
    return () => { live = false }
  }, [vehicles])
  const active = vehicles.filter(v => v.status === 'active')
  const other = vehicles.filter(v => v.status !== 'active')
  const totalDue = Object.values(due).reduce((a, n) => a + n, 0)
  const title = settings?.title || status?.title || 'Pocket Garage'
  const logItems: MenuItem[] = [
    ...(target ? [{ heading: target.display_name }] : []),
    { label: 'Fill-up', icon: 'fuel', hint: 'F', onSelect: () => onLog('fuel') },
    { label: 'Service, repair or expense', icon: 'wrench', hint: 'W', onSelect: () => onLog('record') },
    'sep',
    { label: 'Add a vehicle', icon: 'car', hint: 'N', onSelect: () => onLog('vehicle') },
  ]
  const nav = [
    { to: '/', label: 'Home', icon: 'home', end: true },
    { to: '/vehicles', label: 'Vehicles', icon: 'car', end: true },
    { to: '/reminders', label: 'Reminders', icon: 'bell', badge: totalDue },
  ]
  return (
    <aside className="sidebar" aria-label="Garage">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Garage' ? title : <>Pocket <b>Garage</b></>}</span>
      </div>
      <button type="button" className="btn primary create-btn" aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(menu ? null : rectOf(e.currentTarget))}>
        <Icon name="plus" size={17} strokeWidth={2.2} />Log<Icon name="chevron-down" size={15} className="create-caret" />
      </button>
      {menu && <Menu anchor={menu} items={logItems} onClose={() => setMenu(null)} width={248} />}

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(n => (
          <NavLink key={n.to} to={n.to} end={n.end} onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive ? 'active' : ''}`}>
            <Icon name={n.icon} size={16} />{n.label}{!!n.badge && <span className="badge danger" title={`${n.badge} due or overdue`}>{n.badge}</span>}
          </NavLink>
        ))}
      </nav>

      <VehicleGroup title="My vehicles" list={active} due={due} onLog={onLog} onNavigate={onNavigate} add={() => onLog('vehicle')} />
      {other.length > 0 && <VehicleGroup title="Sold & archived" list={other} due={due} onLog={onLog} onNavigate={onNavigate} closed />}

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('pg:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
        {status?.version && <a className="sb-foot-link sb-ver" href="https://github.com/abjelosevic88/pocket-homelab-garage/releases" target="_blank" rel="noreferrer" title="Release notes">v{status.version.replace(/^v/, '')}</a>}
      </div>
    </aside>
  )
}

function VehicleGroup({ title, list, due, onLog, onNavigate, add, closed }: { title: string; list: Vehicle[]; due: Record<number, number>; onLog: (k: LogKind, v?: Vehicle) => void; onNavigate: () => void; add?: () => void; closed?: boolean }) {
  const [open, setOpen] = useState(!closed)
  return (
    <section className="sb-sec">
      <h4 className="sb-h">
        <button type="button" className="sb-h-btn" aria-expanded={open} onClick={() => setOpen(o => !o)}><Icon name={open ? 'chevron-down' : 'chevron-right'} size={13} />{title}</button>
        {add && <button type="button" className="btn ghost icon-only xs" aria-label="Add a vehicle" title="Add a vehicle (n)" onClick={add}><Icon name="plus" size={14} /></button>}
      </h4>
      {open && (list.length === 0
        ? <p className="sb-empty">No vehicles yet. <button type="button" className="link-btn" onClick={add}>Add one</button></p>
        : <ul className="cal-list">{list.map(v => <VehicleRow key={v.id} v={v} due={due[v.id] || 0} onLog={onLog} onNavigate={onNavigate} />)}</ul>)}
    </section>
  )
}

function VehicleRow({ v, due, onLog, onNavigate }: { v: Vehicle; due: number; onLog: (k: LogKind, v?: Vehicle) => void; onNavigate: () => void }) {
  const navigate = useNavigate()
  const [menu, setMenu] = useState<Anchor | null>(null)
  const go = (tab = '') => { onNavigate(); navigate(`/vehicles/${v.id}${tab ? `/${tab}` : ''}`) }
  const canEdit = v.role !== 'viewer'
  const items: MenuItem[] = [
    { label: 'Open', icon: 'car', onSelect: () => go() },
    ...(canEdit && v.status === 'active' ? [{ label: 'Fill-up', icon: 'fuel', onSelect: () => onLog('fuel', v) }, { label: 'Log work', icon: 'wrench', onSelect: () => onLog('record', v) }] : []),
    'sep',
    { label: 'Fuel', icon: 'fuel', onSelect: () => go('fuel') },
    { label: 'Service & costs', icon: 'wrench', onSelect: () => go('service') },
    { label: 'Reminders', icon: 'bell', onSelect: () => go('reminders') },
    { label: 'Reports', icon: 'chart', onSelect: () => go('reports') },
    ...(canEdit ? ['sep' as const, { label: 'Vehicle settings', icon: 'settings', onSelect: () => go('settings') }] : []),
  ]
  return (
    <li className={`cal-row veh-row ${v.status === 'active' ? '' : 'off'}`}>
      <NavLink to={`/vehicles/${v.id}`} onClick={onNavigate} className={({ isActive }) => `cal-check ${isActive ? 'is-active' : ''}`} title={[v.display_name, v.plate].filter(Boolean).join(' · ')}>
        <VehicleIcon v={v} size={22} />
        <span className="cal-name ellipsis">{v.display_name}</span>
        {due > 0 ? <span className="badge danger veh-due" title={`${due} due or overdue`}>{due}</span> : v.plate ? <span className="veh-meta">{v.plate}</span> : null}
      </NavLink>
      <button type="button" className="btn ghost icon-only xs cal-more" aria-label={`Options for ${v.display_name}`} aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(rectOf(e.currentTarget))}><Icon name="more" size={15} /></button>
      {menu && <Menu anchor={menu} items={items} onClose={() => setMenu(null)} width={210} />}
    </li>
  )
}
