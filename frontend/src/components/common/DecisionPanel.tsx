import { Alert, Button, Descriptions, Form, Input, Modal, Segmented, Space, Tag, Typography } from 'antd'
import { CheckCircleOutlined, StopOutlined, ToolOutlined } from '@ant-design/icons'
import { useMemo, useState } from 'react'
import type { DecisionType, ProductionBatch } from '../../types/domain'
import { BatchStatusBadge } from './BatchStatusBadge'
import { currentRoundSamples, formatReworkRound } from '../../utils/format'

interface Props {
  batch: ProductionBatch
  canDecide: boolean
  loading?: boolean
  onDecide: (decision: DecisionType, reason: string) => Promise<void>
}

export function DecisionPanel({ batch, canDecide, loading, onDecide }: Props) {
  const [open, setOpen] = useState(false)
  const [decision, setDecision] = useState<DecisionType>('release')
  const [form] = Form.useForm<{ reason: string }>()
  const stats = useMemo(() => {
    const round = currentRoundSamples(batch)
    return {
      total: round.length,
      passed: round.filter((item) => item.result === 'pass' && item.retestStatus !== 'requested').length,
      pending: round.filter((item) => item.result === 'pending').length,
      retest: round.filter((item) => item.retestStatus === 'requested').length,
      failed: round.filter((item) => item.result === 'fail').length,
      history: (batch.inspections?.length || 0) - round.length,
    }
  }, [batch])
  const blockers: string[] = []
  if (stats.total === 0) blockers.push('本轮尚未重新登记检验样本')
  if (stats.pending > 0) blockers.push(`有 ${stats.pending} 项待检`)
  if (stats.retest > 0) blockers.push(`有 ${stats.retest} 项待复测`)
  if (stats.failed > 0) blockers.push(`有 ${stats.failed} 项不合格`)
  const releaseBlocked = blockers.length > 0
  const submit = async () => {
    const values = await form.validateFields()
    await onDecide(decision, values.reason)
    setOpen(false)
    form.resetFields()
  }
  return (
    <section className="decision-panel">
      <div className="panel-heading"><div><Typography.Title level={4}>放行判定</Typography.Title><Typography.Text type="secondary">{batch.batchNo}</Typography.Text></div><BatchStatusBadge status={batch.status} /></div>
      <Descriptions column={2} size="small">
        <Descriptions.Item label="当前返工次数">{batch.reworkCount} 次（{formatReworkRound(batch.reworkCount)}）</Descriptions.Item>
        <Descriptions.Item label="本轮样本">{stats.total} 项</Descriptions.Item>
        <Descriptions.Item label="本轮合格">{stats.passed} 项</Descriptions.Item>
        <Descriptions.Item label="待检 / 待复测">{stats.pending} / {stats.retest} 项</Descriptions.Item>
        <Descriptions.Item label="本轮不合格">{stats.failed} 项</Descriptions.Item>
        <Descriptions.Item label="前序轮次记录">{stats.history > 0 ? <Tag>{stats.history} 项（仅供追溯）</Tag> : '无'}</Descriptions.Item>
        <Descriptions.Item label="责任班组">{batch.responsibleTeam}</Descriptions.Item>
      </Descriptions>
      {releaseBlocked
        ? <Alert className="panel-alert" type="warning" showIcon message={`本轮样本不能放行：${blockers.join('、')}`} description="旧轮次检验结果不参与本次放行判断；仍可根据质量情况选择隔离或返工。" />
        : <Alert className="panel-alert" type="success" showIcon message="本轮样本全部合格，可提交放行审批" />}
      <Button type="primary" disabled={!canDecide || batch.status === 'released'} onClick={() => setOpen(true)}>提交决定</Button>
      <Modal title={`审批批次 ${batch.batchNo}`} open={open} confirmLoading={loading} okButtonProps={{ disabled: decision === 'release' && releaseBlocked }}
        onOk={() => void submit()} onCancel={() => setOpen(false)} okText="确认提交" cancelText="取消">
        <Space direction="vertical" size="large" style={{ width: '100%' }}>
          <Descriptions column={2} size="small">
            <Descriptions.Item label="当前返工次数">{batch.reworkCount} 次</Descriptions.Item>
            <Descriptions.Item label="本轮样本处理">{stats.total === 0 ? '暂无样本' : `合格 ${stats.passed} / 待检 ${stats.pending} / 待复测 ${stats.retest} / 不合格 ${stats.failed}`}</Descriptions.Item>
          </Descriptions>
          <Segmented block value={decision} onChange={(value) => setDecision(value as DecisionType)} options={[
            { label: '放行', value: 'release', icon: <CheckCircleOutlined /> },
            { label: '隔离', value: 'quarantine', icon: <StopOutlined /> },
            { label: '返工', value: 'rework', icon: <ToolOutlined /> },
          ]} />
          {decision === 'release' && releaseBlocked && <Alert type="error" showIcon message={`不能放行：${blockers.join('、')}`} />}
          {decision === 'rework' && <Alert type="info" showIcon message="提交返工后返工次数加 1，之后登记的样本才计入下一轮放行判断。" />}
          <Form form={form} layout="vertical"><Form.Item label="审批理由" name="reason" rules={[{ required: true, min: 5, message: '请填写至少 5 个字的审批理由' }]}><Input.TextArea rows={4} maxLength={1000} showCount /></Form.Item></Form>
        </Space>
      </Modal>
    </section>
  )
}
