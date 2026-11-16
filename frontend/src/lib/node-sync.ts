import request from './axios'
import { routingGet } from './routing'
import { requestID } from './request-id'
type Job = { id: number }
export type SyncSubmission = { nodeID: number; jobID?: number; error?: string }

export async function submitNodeSync(ids: number[]): Promise<SyncSubmission[]> {
  const results: SyncSubmission[] = []
  const key = requestID()
  for (const nodeID of [...new Set(ids)]) {
    try {
      const desired = await routingGet<{ desired_hash: string }>(
        `/nodes/${nodeID}/effective-config`
      )
      const res = await request.post<{ data: Job[] }>('/config-deployments', {
        node_ids: [nodeID],
        expected_hashes: { [nodeID]: desired.desired_hash },
        request_key: key,
      })
      results.push({ nodeID, jobID: res.data.data[0].id })
    } catch (e) {
      results.push({ nodeID, error: (e as Error).message })
    }
  }
  return results
}
