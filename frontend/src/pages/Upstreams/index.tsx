import { UpstreamsTable } from '@/components/UpstreamsTable'

/** 上游状态页:反代上游实时请求/失败计数(FR-701)。 */
export default function UpstreamsPage() {
  return <UpstreamsTable refreshInterval={5000} />
}
