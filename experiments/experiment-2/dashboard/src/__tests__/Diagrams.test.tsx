// Smoke tests: verify diagrams render without throwing and produce SVG output.

import { render } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { CoreDiagram }       from '../components/diagrams/CoreDiagram'
import { SupportDiagram }    from '../components/diagrams/SupportDiagram'
import { ExperimentDiagram } from '../components/diagrams/ExperimentDiagram'
import { DiagramsView }      from '../views/DiagramsView'

describe('CoreDiagram', () => {
  it('renders an SVG element', () => {
    const { container } = render(<CoreDiagram />)
    expect(container.querySelector('svg')).not.toBeNull()
  })

  it('contains system labels', () => {
    const { getByText } = render(<CoreDiagram />)
    expect(getByText('ServiceRegistry')).toBeTruthy()
    expect(getByText('DynamicOrch')).toBeTruthy()
    expect(getByText('ConsumerAuth')).toBeTruthy()
    expect(getByText('CA')).toBeTruthy()
  })
})

describe('SupportDiagram', () => {
  it('renders an SVG element', () => {
    const { container } = render(<SupportDiagram />)
    expect(container.querySelector('svg')).not.toBeNull()
  })

  it('contains RabbitMQ label', () => {
    const { getAllByText } = render(<SupportDiagram />)
    expect(getAllByText(/RabbitMQ/).length).toBeGreaterThan(0)
  })
})

describe('ExperimentDiagram', () => {
  it('renders an SVG element', () => {
    const { container } = render(<ExperimentDiagram />)
    expect(container.querySelector('svg')).not.toBeNull()
  })

  it('shows all major components', () => {
    const { getByText } = render(<ExperimentDiagram />)
    expect(getByText('robot-fleet')).toBeTruthy()
    expect(getByText('edge-adapter')).toBeTruthy()
    expect(getByText('consumer')).toBeTruthy()
    expect(getByText('DynamicOrch')).toBeTruthy()
    expect(getByText('ServiceReg')).toBeTruthy()
  })
})

describe('DiagramsView', () => {
  it('renders the Experiment 2 diagram by default', () => {
    const { getByText } = render(<DiagramsView />)
    // Heading present
    expect(getByText('System Diagrams')).toBeTruthy()
    // Default tab is Experiment 2
    expect(getByText('robot-fleet')).toBeTruthy()
  })

  it('switches to Core Systems diagram', async () => {
    const { getByRole, findByText } = render(<DiagramsView />)
    getByRole('button', { name: 'Core Systems' }).click()
    expect(await findByText('ServiceRegistry')).toBeTruthy()
  })
})
