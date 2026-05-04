import { createContext, useContext, useState, useCallback, type ReactNode } from 'react'
import type { DashboardConfig } from './types'
import { loadConfig, saveConfig, resetConfig } from './storage'

interface ConfigContextValue {
  config:          DashboardConfig
  setConfig:       (cfg: DashboardConfig) => void
  resetToDefaults: () => void
}

const ConfigContext = createContext<ConfigContextValue | null>(null)

export function ConfigProvider({ children }: { children: ReactNode }) {
  const [config, setConfigState] = useState<DashboardConfig>(loadConfig)

  const setConfig = useCallback((cfg: DashboardConfig) => {
    saveConfig(cfg)
    setConfigState(cfg)
  }, [])

  const resetToDefaults = useCallback(() => {
    setConfigState(resetConfig())
  }, [])

  return (
    <ConfigContext.Provider value={{ config, setConfig, resetToDefaults }}>
      {children}
    </ConfigContext.Provider>
  )
}

export function useConfig(): ConfigContextValue {
  const ctx = useContext(ConfigContext)
  if (!ctx) throw new Error('useConfig must be used inside <ConfigProvider>')
  return ctx
}
