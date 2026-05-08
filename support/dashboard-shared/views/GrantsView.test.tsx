// Tests for GrantsView.
//
// GrantsPanel is experiment-specific and is mocked for isolation.

import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { GrantsView } from './GrantsView'

vi.mock('../components/GrantsPanel', () => ({
  GrantsPanel: () => <div data-testid="grants-panel" />,
}))

describe('GrantsView', () => {
  it('renders without crashing', () => {
    render(<GrantsView />)
  })

  it('renders the GrantsPanel', () => {
    render(<GrantsView />)
    expect(screen.getByTestId('grants-panel')).toBeInTheDocument()
  })
})
