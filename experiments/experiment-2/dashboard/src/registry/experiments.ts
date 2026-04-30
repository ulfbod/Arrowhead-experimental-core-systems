// Experiment registry — allows new experiment views to be added without
// modifying the navigation shell.  Each experiment registers itself here;
// the NavBar and App shell enumerate the registry at render time.

import type { ComponentType } from 'react'

export interface ExperimentDef {
  id: string
  label: string
  description: string
  component: ComponentType
}

const registry: ExperimentDef[] = []

export function registerExperiment(def: ExperimentDef): void {
  if (registry.some(e => e.id === def.id)) {
    throw new Error(`Experiment "${def.id}" is already registered`)
  }
  registry.push(def)
}

export function getExperiments(): readonly ExperimentDef[] {
  return registry
}
