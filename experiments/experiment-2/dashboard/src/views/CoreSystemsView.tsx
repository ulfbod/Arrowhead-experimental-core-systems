import { SystemHealthGrid } from '../components/SystemHealthGrid'
import { ServiceList } from '../components/ServiceList'
import { AuthRulesList } from '../components/AuthRulesList'

export function CoreSystemsView() {
  return (
    <div>
      <SystemHealthGrid />
      <ServiceList />
      <AuthRulesList />
    </div>
  )
}
