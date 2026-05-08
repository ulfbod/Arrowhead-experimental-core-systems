// Tests for LiveDataView.
//
// ConsumerStatsPanel is experiment-specific and is mocked for isolation.

import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { LiveDataView } from './LiveDataView'

vi.mock('../components/ConsumerStatsPanel', () => ({
  ConsumerStatsPanel: () => <div data-testid="consumer-stats-panel" />,
}))

describe('LiveDataView', () => {
  it('renders without crashing', () => {
    render(<LiveDataView />)
  })

  it('renders the ConsumerStatsPanel', () => {
    render(<LiveDataView />)
    expect(screen.getByTestId('consumer-stats-panel')).toBeInTheDocument()
  })
})
