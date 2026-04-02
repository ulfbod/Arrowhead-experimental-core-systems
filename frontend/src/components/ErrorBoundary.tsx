import React from 'react'

interface Props { children: React.ReactNode; name?: string }
interface State { error: Error | null }

export class ErrorBoundary extends React.Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  render() {
    if (this.state.error) {
      return (
        <div style={{
          margin: 24,
          padding: 20,
          background: '#fee2e2',
          border: '1px solid #fca5a5',
          borderRadius: 8,
          color: '#991b1b',
        }}>
          <div style={{ fontWeight: 700, marginBottom: 8 }}>
            {this.props.name ?? 'Component'} crashed
          </div>
          <pre style={{ fontSize: '0.75rem', whiteSpace: 'pre-wrap', color: '#7f1d1d' }}>
            {this.state.error.message}
          </pre>
          <button
            style={{ marginTop: 12, padding: '6px 14px', background: 'var(--red)', border: 'none', borderRadius: 4, color: '#fff', cursor: 'pointer' }}
            onClick={() => this.setState({ error: null })}
          >
            Retry
          </button>
        </div>
      )
    }
    return this.props.children
  }
}
