// Shows the actual RabbitMQ managed users and their topic permissions.

import { usePolling } from '../hooks/usePolling'
import { fetchRabbitUsers, fetchTopicPermissions } from '../api'
import type { RabbitUser, RabbitTopicPermission } from '../types'

const MANAGED_TAG = 'arrowhead-managed'

function isManagedUser(u: RabbitUser): boolean {
  return u.tags.includes(MANAGED_TAG)
}

export function TopicSyncPanel() {
  const { data: users, error: usersErr, stale: usersStale } =
    usePolling(fetchRabbitUsers, 5_000)
  const { data: perms, error: permsErr, stale: permsStale } =
    usePolling(fetchTopicPermissions, 5_000)

  const managedUsers: RabbitUser[] = (users ?? []).filter(isManagedUser)

  // Build a map: username → topic permissions on the arrowhead exchange.
  const permMap = new Map<string, RabbitTopicPermission>()
  for (const p of perms ?? []) {
    if (p.exchange === 'arrowhead') permMap.set(p.user, p)
  }

  const stale = usersStale || permsStale
  const error = usersErr ?? permsErr

  return (
    <section style={s.section}>
      <h2 style={s.heading}>
        RabbitMQ Managed Users
        {stale && <span style={s.stale}> (stale)</span>}
      </h2>
      <p style={s.hint}>
        Users tagged <code>arrowhead-managed</code> were provisioned by a previous sync run.
        Authorization is now enforced live by topic-auth-http on every broker operation.
      </p>

      {error && !stale && <p style={s.err}>{error}</p>}

      {managedUsers.length === 0
        ? <p style={s.dim}>No managed users — authorization is enforced live by topic-auth-http.</p>
        : (
          <table style={s.table}>
            <thead>
              <tr>
                {['User', 'Exchange', 'Read pattern (routing key)', 'Write pattern'].map(h => (
                  <th key={h} style={s.th}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {managedUsers.map(u => {
                const perm = permMap.get(u.name)
                return (
                  <tr key={u.name}>
                    <td style={s.td}>{u.name}</td>
                    <td style={s.td}>{perm?.exchange ?? '—'}</td>
                    <td style={{ ...s.td, ...s.mono }}>{perm?.read || <em style={s.dim}>none</em>}</td>
                    <td style={{ ...s.td, ...s.mono }}>{perm?.write || <em style={s.dim}>none</em>}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        )
      }
    </section>
  )
}

const s: Record<string, React.CSSProperties> = {
  section: { marginBottom: 28 },
  heading: { fontSize: '0.9rem', marginBottom: 6, color: '#555' },
  hint:    { color: '#888', fontSize: '0.75rem', marginBottom: 8 },
  table:   { width: '100%', borderCollapse: 'collapse', fontSize: '0.8rem' },
  th:      { textAlign: 'left', padding: '4px 8px', borderBottom: '2px solid #ddd' },
  td:      { padding: '4px 8px', borderBottom: '1px solid #eee' },
  mono:    { fontFamily: 'monospace', color: '#1a5276' },
  err:     { color: '#f44336', fontSize: '0.8rem' },
  dim:     { color: '#999', fontSize: '0.8rem' },
  stale:   { color: '#ff9800', fontWeight: 'normal', fontSize: '0.75rem' },
}
