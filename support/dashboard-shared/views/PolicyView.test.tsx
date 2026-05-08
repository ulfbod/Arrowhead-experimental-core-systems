// Tests for PolicyView.
//
// PolicyProjectionPanel is experiment-specific and is mocked for isolation.

import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { PolicyView } from './PolicyView'

vi.mock('../components/PolicyProjectionPanel', () => ({
  PolicyProjectionPanel: () => <div data-testid="policy-projection-panel" />,
}))

describe('PolicyView', () => {
  it('renders without crashing', () => {
    render(<PolicyView />)
  })

  it('renders the PolicyProjectionPanel', () => {
    render(<PolicyView />)
    expect(screen.getByTestId('policy-projection-panel')).toBeInTheDocument()
  })
})
