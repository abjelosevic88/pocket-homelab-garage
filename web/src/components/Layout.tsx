// The app shell: sidebar, top bar, search palette, shortcuts — the Pocket Calendar layout.
import { useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useMatch, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { Vehicle } from '../lib/types'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { ShortcutsSheet } from './ShortcutsSheet'
import { FuelModal, RecordModal, VehicleModal } from './forms'
import { Avatar, type MenuItem, MenuButton, modalOpen, useIsMobile, useMediaQuery } from './ui'
import { AskButton } from './AskButton'

const SIDEBAR_KEY = 'pg-sidebar'

/** Something was logged or changed from the shell: pages reload. */
export const announceChange = () => window.dispatchEvent(new CustomEvent('pg:changed'))
export function useOnChange(cb: () => void) {
  const ref = useRef(cb)
  ref.current = cb
  useEffect(() => {
    const h = () => ref.current()
    window.addEventListener('pg:changed', h)
    return () => window.removeEventListener('pg:changed', h)
  }, [])
}

export type LogKind = 'fuel' | 'record' | 'vehicle'

export default function Layout() {
  const { vehicles, reloadVehicles } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  useEffect(() => { document.querySelector('.content')?.scrollTo({ top: 0 }) }, [loc.pathname])

  const [palette, setPalette] = useState(false)
  const [shortcuts, setShortcuts] = useState(false)
  const [modal, setModal] = useState<{ kind: LogKind; vehicle?: Vehicle } | null>(null)

  // the vehicle on screen, else the first active one
  const vm = useMatch('/vehicles/:id/*')
  const current = vehicles.find(v => String(v.id) === vm?.params.id)
  const active = vehicles.filter(v => v.status === 'active')
  const target = current?.status === 'active' ? current : active[0]

  /** Open a log form; the forms want the vehicle with its summary (odometer estimate). */
  const log = async (kind: LogKind, v?: Vehicle) => {
    closeDrawer()
    if (kind === 'vehicle') { setModal({ kind }); return }
    const veh = v || target
    if (!veh) { setModal({ kind: 'vehicle' }); return }
    const full = await api.get<Vehicle>(`${V1}/vehicles/${veh.id}`).catch(() => veh)
    setModal({ kind, vehicle: full })
  }
  const logRef = useRef(log)
  logRef.current = log

  useEffect(() => {
    const open = () => setShortcuts(true)
    window.addEventListener('pg:shortcuts', open)
    return () => window.removeEventListener('pg:shortcuts', open)
  }, [])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t.closest('input, textarea, select, [contenteditable="true"]') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      const go = (to: string) => { e.preventDefault(); navigate(to) }
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); setShortcuts(true); break
        case 'f': e.preventDefault(); logRef.current('fuel'); break
        case 'w': e.preventDefault(); logRef.current('record'); break
        case 'n': e.preventDefault(); logRef.current('vehicle'); break
        case 'h': go('/'); break
        case 'v': go('/vehicles'); break
        case 'r': go('/reminders'); break
        case ',': go('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const commands: PaletteCommand[] = [
    { id: 'fuel', label: target ? `Log a fill-up — ${target.display_name}` : 'Log a fill-up', icon: 'fuel', hint: 'F', keywords: 'fuel tank petrol diesel gas', run: () => log('fuel') },
    { id: 'work', label: target ? `Log work — ${target.display_name}` : 'Log work', icon: 'wrench', hint: 'W', keywords: 'service repair upgrade expense record', run: () => log('record') },
    { id: 'vehicle', label: 'Add a vehicle', icon: 'plus', hint: 'N', keywords: 'new car', run: () => log('vehicle') },
    { id: 'home', label: 'Home', icon: 'home', hint: 'H', run: () => navigate('/') },
    { id: 'vehicles', label: 'Vehicles', icon: 'car', hint: 'V', run: () => navigate('/vehicles') },
    { id: 'reminders', label: 'Reminders', icon: 'bell', hint: 'R', run: () => navigate('/reminders') },
    { id: 'quick', label: 'Quick log (phone page)', icon: 'smartphone', keywords: 'quick log', run: () => navigate('/quick') },
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: () => setShortcuts(true) },
  ]

  const saved = () => { setModal(null); reloadVehicles(); announceChange() }
  return (
    <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
      {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
      <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
        <Sidebar onLog={log} onNavigate={closeDrawer} target={target} />
      </div>
      <div className="main">
        <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} vehicle={current} />
        <main className="content"><div className="content-inner"><Outlet /></div></main>
      </div>
      {mobile && <button type="button" className="fab" aria-label="Log a fill-up" title="Log a fill-up" onClick={() => log('fuel')}><Icon name="fuel" size={24} strokeWidth={2} /></button>}
      {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
      {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
      {modal?.kind === 'fuel' && modal.vehicle && <FuelModal vehicle={modal.vehicle} onClose={() => setModal(null)} onSaved={saved} />}
      {modal?.kind === 'record' && modal.vehicle && <RecordModal vehicle={modal.vehicle} onClose={() => setModal(null)} onSaved={saved} />}
      {modal?.kind === 'vehicle' && <VehicleModal onClose={() => setModal(null)} onSaved={v => { setModal(null); reloadVehicles(); navigate(`/vehicles/${v.id}`) }} />}
    </div>
  )
}

const SECTION: Record<string, string> = { '': 'Home', vehicles: 'Vehicles', reminders: 'Reminders' }

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen, vehicle }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean; vehicle?: Vehicle }) {
  const { user, themeMode, setThemeMode, status, setUser } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const section = SECTION[loc.pathname.split('/')[1] || ''] || ''
  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: 'Profile', icon: 'user', onSelect: () => navigate('/settings/profile') },
    { label: 'Settings', icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: 'Keyboard shortcuts', icon: 'keyboard', onSelect: () => window.dispatchEvent(new CustomEvent('pg:shortcuts')), hint: '?' },
    'sep',
    { heading: 'Appearance' },
    { label: 'System', checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: 'Light', checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: 'Dark', checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: 'Sign out', icon: 'log-out', onSelect: signOut }] : []),
  ]
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? 'Hide the sidebar' : 'Show the sidebar'} title="Menu ([)"><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title">
        {vehicle ? <><button type="button" className="crumb" onClick={() => navigate('/vehicles')}>Vehicles</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{vehicle.display_name}</span></> : section}
      </h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label="Search" title="Search (⌘K or /)"><Icon name="search" size={18} /></button>
      {mobile && <AskButton />}
      <MenuButton label="Account" className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} />
      </MenuButton>
    </header>
  )
}
