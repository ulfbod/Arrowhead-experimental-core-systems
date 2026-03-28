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
          background: 'rgba(244,67,54,0.08)',
          border: '1px solid var(--red)',
          borderRadius: 8,
          color: 'var(--red-light)',
        }}>
          <div style={{ fontWeight: 700, marginBottom: 8 }}>
            {this.props.name ?? 'Component'} crashed
          </div>
          <pre style={{ fontSize: '0.75rem', whiteSpace: 'pre-wrap', color: 'var(--text-muted)' }}>
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
