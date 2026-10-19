import React, { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { api, V1 } from './api'
import { setDateFormats, type DateFormat, type TimeFormat } from './format'
import type { Currency, Meta, Settings, User, Vehicle } from './types'

interface AuthStatus { needs_setup: boolean; auth_mode: string; authenticated: boolean; user: User | null; version: string; title: string; oidc?: boolean; oidc_name?: string }
interface AppState {
  status: AuthStatus | null
  user: User | null
  settings: Settings | null
  users: User[]
  currencies: Currency[]
  meta: Meta | null
  vehicles: Vehicle[]
  theme: 'light' | 'dark'
  toggleTheme: () => void
  refresh: () => Promise<void>
  reloadMeta: () => Promise<void>
  reloadVehicles: () => Promise<void>
  setUser: (u: User | null) => void
  setSettings: (s: Settings) => void
}
const Ctx = createContext<AppState>(null!)
export function useApp() { return useContext(Ctx) }

const EMPTY_META: Meta = { vehicle_kinds: [], fuel_types: [], record_kinds: [], plan_stages: [], priorities: [], document_kinds: [], service_categories: [], expense_categories: [], reminder_templates: [] }

export function AppProvider({ children }: { children: React.ReactNode }) {
  const [status, setStatus] = useState<AuthStatus | null>(null)
  const [user, setUser] = useState<User | null>(null)
  const [settings, setSettingsRaw] = useState<Settings | null>(null)
  const [users, setUsers] = useState<User[]>([])
  const [currencies, setCurrencies] = useState<Currency[]>([])
  const [meta, setMeta] = useState<Meta | null>(null)
  const [vehicles, setVehicles] = useState<Vehicle[]>([])
  const [theme, setTheme] = useState<'light' | 'dark'>(() => (localStorage.getItem('pg-theme') as 'light' | 'dark') || (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'))
  useEffect(() => { document.documentElement.dataset.theme = theme; localStorage.setItem('pg-theme', theme) }, [theme])
  useEffect(() => { if (settings?.title) document.title = settings.title }, [settings?.title])
  const setSettings = useCallback((s: Settings) => { setSettingsRaw(s); setDateFormats(s.date_format as DateFormat, s.time_format as TimeFormat) }, [])
  const reloadVehicles = useCallback(async () => { setVehicles(await api.get<Vehicle[]>(`${V1}/vehicles?bare=1`)) }, [])
  const reloadMeta = useCallback(async () => {
    const [u, c, m] = await Promise.all([api.get<User[]>(`${V1}/users`), api.get<Currency[]>(`${V1}/currencies`), api.get<Meta>(`${V1}/meta`).catch(() => EMPTY_META)])
    setUsers(u); setCurrencies(c); setMeta(m)
    await reloadVehicles()
  }, [reloadVehicles])
  const refresh = useCallback(async () => {
    const st = await api.get<AuthStatus>(`${V1}/auth/status`)
    setStatus(st)
    setUser(st.user)
    if (st.authenticated) {
      setSettings(await api.get<Settings>(`${V1}/settings`))
      await reloadMeta()
    }
  }, [reloadMeta, setSettings])
  useEffect(() => { refresh().catch(() => setStatus({ needs_setup: false, auth_mode: 'local', authenticated: false, user: null, version: '', title: 'Pocket Garage' })) }, [refresh])
  useEffect(() => {
    const h = () => { setUser(null); setStatus(s => s ? { ...s, authenticated: false, user: null } : s) }
    window.addEventListener('pg:unauthorized', h)
    return () => window.removeEventListener('pg:unauthorized', h)
  }, [])
  return <Ctx.Provider value={{ status, user, settings, users, currencies, meta, vehicles, theme, toggleTheme: () => setTheme(t => t === 'dark' ? 'light' : 'dark'), refresh, reloadMeta, reloadVehicles, setUser, setSettings }}>{children}</Ctx.Provider>
}
