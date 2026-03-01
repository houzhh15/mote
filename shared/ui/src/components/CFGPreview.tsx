import React from 'react';
import { Badge, Card, Space, Tag, Typography } from 'antd';
import { CheckCircleOutlined, CloseCircleOutlined, WarningOutlined } from '@ant-design/icons';
import type { Step, ValidationResult } from '../types';

const { Text, Paragraph } = Typography;

interface CFGPreviewProps {
  steps: Step[];
  agentName: string;
  validationResults?: ValidationResult[];
  onValidate?: () => void;
}

export const CFGPreview: React.FC<CFGPreviewProps> = ({
  steps,
  agentName,
  validationResults,
  onValidate,
}) => {
  const errors = validationResults?.filter((r) => r.level === 'error') || [];
  const warnings = validationResults?.filter((r) => r.level === 'warning') || [];

  const statusColor = errors.length > 0 ? 'red' : warnings.length > 0 ? 'gold' : 'green';
  const statusIcon =
    errors.length > 0 ? (
      <CloseCircleOutlined />
    ) : warnings.length > 0 ? (
      <WarningOutlined />
    ) : (
      <CheckCircleOutlined />
    );
  const statusText =
    errors.length > 0
      ? `${errors.length} 个错误`
      : warnings.length > 0
        ? `${warnings.length} 个警告`
        : '配置有效';

  return (
    <Card
      size="small"
      title={
        <Space>
          <Text strong>{agentName}</Text>
          <Badge
            count={
              <Tag
                color={statusColor}
                icon={statusIcon}
                style={{ cursor: onValidate ? 'pointer' : undefined }}
                onClick={onValidate}
              >
                {statusText}
              </Tag>
            }
          />
        </Space>
      }
    >
      {/* YAML-like preview */}
      <pre style={{ fontSize: 12, background: '#f5f5f5', padding: 12, borderRadius: 4, overflow: 'auto', maxHeight: 400 }}>
        {`agent: ${agentName}\nsteps:\n`}
        {steps.map((step, _i) => {
          let yaml = `  - type: ${step.type}\n`;
          if (step.label) yaml += `    label: ${step.label}\n`;
          if (step.type === 'prompt' && step.content) {
            yaml += `    content: |\n      ${step.content.replace(/\n/g, '\n      ')}\n`;
          }
          if (step.type === 'agent_ref' && step.agent) {
            yaml += `    agent: ${step.agent}\n`;
          }
          if (step.type === 'route') {
            if (step.prompt) {
              yaml += `    prompt: ${step.prompt}\n`;
            }
            if (step.branches && Object.keys(step.branches).length > 0) {
              yaml += `    branches:\n`;
              for (const [match, target] of Object.entries(step.branches)) {
                yaml += `      ${match}: ${target}\n`;
              }
            }
          }
          return yaml;
        }).join('')}
      </pre>

      {/* Validation results */}
      {validationResults && validationResults.length > 0 && (
        <div style={{ marginTop: 8 }}>
          {validationResults.map((r, i) => (
            <Paragraph
              key={i}
              type={r.level === 'error' ? 'danger' : 'warning'}
              style={{ margin: '4px 0', fontSize: 12 }}
            >
              <Tag color={r.level === 'error' ? 'error' : 'warning'} style={{ fontSize: 10 }}>
                {r.code}
              </Tag>
              {r.step_index >= 0 && (
                <Text type="secondary" style={{ fontSize: 11 }}>
                  步骤 #{r.step_index + 1}{' '}
                </Text>
              )}
              {r.message}
            </Paragraph>
          ))}
        </div>
      )}
    </Card>
  );
};
