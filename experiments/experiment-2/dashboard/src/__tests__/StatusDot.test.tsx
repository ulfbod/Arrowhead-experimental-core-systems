import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { StatusDot } from '../components/StatusDot'

describe('StatusDot', () => {
  it('renders a green dot for ok', () => {
    render(<StatusDot status="ok" />)
    expect(screen.getByRole('img', { name: 'ok' })).toHaveAttribute('data-status', 'ok')
  })

  it('renders a red dot for error', () => {
    render(<StatusDot status="error" />)
    expect(screen.getByRole('img', { name: 'error' })).toHaveAttribute('data-status', 'error')
  })

  it('renders a grey dot for loading', () => {
    render(<StatusDot status="loading" />)
    expect(screen.getByRole('img', { name: 'loading' })).toHaveAttribute('data-status', 'loading')
  })
})
