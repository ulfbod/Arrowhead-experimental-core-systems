import { GrantsPanel }    from '../components/GrantsPanel'
import { TopicSyncPanel } from '../components/TopicSyncPanel'

export function GrantsView() {
  return (
    <div>
      <GrantsPanel />
      <hr style={s.divider} />
      <TopicSyncPanel />
    </div>
  )
}

const s: Record<string, React.CSSProperties> = {
  divider: { border: 'none', borderTop: '1px solid #eee', margin: '8px 0 24px' },
}
