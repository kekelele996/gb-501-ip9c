import { Alert, Button, Descriptions, Empty, Form, Input, Modal, Segmented, Space, Table, Tag, Typography } from 'antd'
import { CheckCircleOutlined, StopOutlined, ToolOutlined } from '@ant-design/icons'
import { useState } from 'react'
import type { ColumnsType } from 'antd/es/table'
import type { DecisionType, InspectionSample, ProductionBatch } from '../../types/domain'
import { BatchStatusBadge } from './BatchStatusBadge'
import { StatusBadge } from './StatusBadge'
import { currentRoundSamples, roundLabel, summarizeRound } from '../../utils/rework'

interface Props {
  batch: ProductionBatch
  canDecide: boolean
  loading?: boolean
  onDecide: (decision: DecisionType, reason: string) => Promise<void>
}

const sampleColumns: ColumnsType<InspectionSample> = [
  { title: '样本编号', dataIndex: 'sampleCode' },
  { title: '抽样位置', dataIndex: 'samplingPosition' },
  { title: '检验项', dataIndex: 'inspectionItem' },
  { title: '结果', dataIndex: 'result', render: (value) => <StatusBadge value={value} /> },
  { title: '复测', dataIndex: 'retestStatus', render: (value) => <StatusBadge value={value} /> },
]

export function DecisionPanel({ batch, canDecide, loading, onDecide }: Props) {
  const [open, setOpen] = useState(false)
  const [decision, setDecision] = useState<DecisionType>('release')
  const [form] = Form.useForm<{ reason: string }>()
  const stats = summarizeRound(batch)
  const currentSamples = currentRoundSamples(batch)
  const archived = (batch.inspections?.length || 0) - stats.total
  const releaseBlockers: string[] = []
  if (stats.total === 0) releaseBlockers.push('本轮尚未登记新的检验样本')
  if (stats.pending > 0) releaseBlockers.push(`有 ${stats.pending} 项样本待检验`)
  if (stats.retest > 0) releaseBlockers.push(`有 ${stats.retest} 项样本待复测`)
  if (stats.failed > 0) releaseBlockers.push(`有 ${stats.failed} 项样本不合格`)
  const submit = async () => {
    const values = await form.validateFields()
    await onDecide(decision, values.reason)
    setOpen(false)
    form.resetFields()
  }
  return (
    <section className="decision-panel">
      <div className="panel-heading">
        <div><Typography.Title level={4}>放行判定</Typography.Title><Typography.Text type="secondary">{batch.batchNo}</Typography.Text></div>
        <Space size="small"><Tag color={batch.reworkRound > 0 ? 'warning' : 'default'}>{roundLabel(batch.reworkRound)}</Tag><BatchStatusBadge status={batch.status} /></Space>
      </div>
      <Descriptions column={3} size="small">
        <Descriptions.Item label="本轮样本">{stats.total}</Descriptions.Item>
        <Descriptions.Item label="合格">{stats.passed}</Descriptions.Item>
        <Descriptions.Item label="待检验">{stats.pending}</Descriptions.Item>
        <Descriptions.Item label="待复测">{stats.retest}</Descriptions.Item>
        <Descriptions.Item label="不合格">{stats.failed}</Descriptions.Item>
        <Descriptions.Item label="责任班组">{batch.responsibleTeam}</Descriptions.Item>
      </Descriptions>
      <div className="round-sample-list">
        <Typography.Text strong>{roundLabel(batch.reworkRound)}的检验样本</Typography.Text>
        {currentSamples.length > 0 ? (
          <Table<InspectionSample> size="small" rowKey="id" pagination={false} columns={sampleColumns} dataSource={currentSamples} />
        ) : (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="本轮还没有登记检验样本，不能放行" />
        )}
        {archived > 0 && <Typography.Text type="secondary">另有 {archived} 条往轮检验记录保留在检验明细中，仅供追溯，不参与本次放行判断。</Typography.Text>}
      </div>
      {releaseBlockers.length > 0 && (
        <Alert
          className="panel-alert"
          type={decision === 'release' ? 'error' : 'info'}
          showIcon
          message={decision === 'release' ? '当前不能放行' : '不影响隔离 / 返工决定'}
          description={
            <ul className="blocker-list">
              {releaseBlockers.map((blocker) => <li key={blocker}>{blocker}</li>)}
            </ul>
          }
        />
      )}
      <Button type="primary" disabled={!canDecide || batch.status === 'released'} onClick={() => setOpen(true)}>提交决定</Button>
      <Modal title={`审批批次 ${batch.batchNo}`} open={open} confirmLoading={loading} onOk={() => void submit()} onCancel={() => setOpen(false)} okText="确认提交" cancelText="取消">
        <Space direction="vertical" size="large" style={{ width: '100%' }}>
          <Space size="small"><Tag color={batch.reworkRound > 0 ? 'warning' : 'default'}>{roundLabel(batch.reworkRound)}</Tag><Typography.Text type="secondary">仅本轮样本参与放行判断</Typography.Text></Space>
          <Segmented block value={decision} onChange={(value) => setDecision(value as DecisionType)} options={[
            { label: '放行', value: 'release', icon: <CheckCircleOutlined /> },
            { label: '隔离', value: 'quarantine', icon: <StopOutlined /> },
            { label: '返工', value: 'rework', icon: <ToolOutlined /> },
          ]} />
          {decision === 'release' && releaseBlockers.length > 0 && <Alert type="error" showIcon message="本轮样本未全部闭环，提交放行会被系统拒绝" description={releaseBlockers.join('；')} />}
          <Form form={form} layout="vertical"><Form.Item label="审批理由" name="reason" rules={[{ required: true, min: 5, message: '请填写至少 5 个字的审批理由' }]}><Input.TextArea rows={4} maxLength={1000} showCount /></Form.Item></Form>
        </Space>
      </Modal>
    </section>
  )
}
