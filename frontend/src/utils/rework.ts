import type { InspectionSample, ProductionBatch } from '../types/domain'

// 首轮生产记为第 0 轮；每进入一次返工，批次 reworkRound 加 1。
export const currentRoundSamples = (batch: ProductionBatch): InspectionSample[] =>
  (batch.inspections || []).filter((sample) => sample.reworkRound === batch.reworkRound)

export const roundLabel = (round: number): string => (round <= 0 ? '首轮生产' : `第 ${round} 次返工`)

export const shortRoundLabel = (round: number): string => (round <= 0 ? '首轮' : `返工 ${round}`)

export interface RoundStats {
  total: number
  passed: number
  failed: number
  pending: number
  retest: number
}

export const summarizeRound = (batch: ProductionBatch): RoundStats => {
  const stats: RoundStats = { total: 0, passed: 0, failed: 0, pending: 0, retest: 0 }
  for (const sample of currentRoundSamples(batch)) {
    stats.total++
    if (sample.result === 'pass') stats.passed++
    else if (sample.result === 'fail') stats.failed++
    else stats.pending++
    if (sample.retestStatus === 'requested') stats.retest++
  }
  return stats
}
