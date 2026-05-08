// Tests for the StatusDot component.

import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { StatusDot } from './StatusDot'

describe('StatusDot — accessibility', () => {
  it('renders with role="img"', () => {
    render(<StatusDot status="ok" />)
    expect(screen.getByRole('img')).toBeInTheDocument()
  })

  it('aria-label matches the status prop', () => {
    const { rerender } = render(<StatusDot status="ok" />)
    expect(screen.getByRole('img', { name: 'ok' })).toBeInTheDocument()

    rerender(<StatusDot status="error" />)
    expect(screen.getByRole('img', { name: 'error' })).toBeInTheDocument()

    rerender(<StatusDot status="loading" />)
    expect(screen.getByRole('img', { name: 'loading' })).toBeInTheDocument()
  })
})

describe('StatusDot — data attribute', () => {
  it('sets data-status to the given status', () => {
    const { rerender } = render(<StatusDot status="ok" />)
    expect(screen.getByRole('img')).toHaveAttribute('data-status', 'ok')

    rerender(<StatusDot status="error" />)
    expect(screen.getByRole('img')).toHaveAttribute('data-status', 'error')

    rerender(<StatusDot status="loading" />)
    expect(screen.getByRole('img')).toHaveAttribute('data-status', 'loading')
  })
})

describe('StatusDot — colours', () => {
  it('is green for ok status', () => {
    render(<StatusDot status="ok" />)
    expect(screen.getByRole('img')).toHaveStyle({ background: '#4caf50' })
  })

  it('is red for error status', () => {
    render(<StatusDot status="error" />)
    expect(screen.getByRole('img')).toHaveStyle({ background: '#f44336' })
  })

  it('is grey for loading status', () => {
    render(<StatusDot status="loading" />)
    expect(screen.getByRole('img')).toHaveStyle({ background: '#9e9e9e' })
  })
})

describe('StatusDot — shape', () => {
  it('is circular', () => {
    render(<StatusDot status="ok" />)
    expect(screen.getByRole('img')).toHaveStyle({ borderRadius: '50%' })
  })
})
