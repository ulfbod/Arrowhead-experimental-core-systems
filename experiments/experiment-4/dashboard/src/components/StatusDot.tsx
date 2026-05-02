interface Props {
  status: 'ok' | 'error' | 'loading'
}

const COLOR: Record<Props['status'], string> = {
  ok:      '#4caf50',
  error:   '#f44336',
  loading: '#9e9e9e',
}

export function StatusDot({ status }: Props) {
  return (
    <span
      role="img"
      aria-label={status}
      data-status={status}
      style={{
        display: 'inline-block',
        width: 10, height: 10,
        borderRadius: '50%',
        background: COLOR[status],
        marginRight: 6,
        flexShrink: 0,
      }}
    />
  )
}
