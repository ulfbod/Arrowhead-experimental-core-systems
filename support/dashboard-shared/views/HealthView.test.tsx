// Tests for HealthView.
//
// SystemHealthGrid is experiment-specific (not in dashboard-shared) and is
// mocked so the view can be tested in isolation.

import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { HealthView } from './HealthView'

vi.mock('../components/SystemHealthGrid', () => ({
  SystemHealthGrid: () => <div data-testid="system-health-grid" />,
}))

describe('HealthView', () => {
  it('renders without crashing', () => {
    render(<HealthView />)
  })

  it('renders the SystemHealthGrid', () => {
    render(<HealthView />)
    expect(screen.getByTestId('system-health-grid')).toBeInTheDocument()
  })
})
