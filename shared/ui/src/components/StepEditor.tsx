import React, { useCallback, useMemo } from 'react';
import { Button, Card, Input, Select, Space, Tag, Typography } from 'antd';
import {
  DeleteOutlined,
  PlusOutlined,
  HolderOutlined,
  ApiOutlined,
  MessageOutlined,
  BranchesOutlined,
} from '@ant-design/icons';
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import type { AgentOption, Step, StepType } from '../types';
import { RouterConfig } from './RouterConfig';

const { TextArea } = Input;
const { Text } = Typography;

interface StepEditorProps {
  steps: Step[];
  onChange: (steps: Step[]) => void;
  availableAgents: AgentOption[];
  /** Called when user clicks "+ 新建" on an agent_ref step; opens the create-agent flow */
  onCreateAgent?: () => void;
}

const STEP_TYPE_OPTIONS: { value: StepType; label: string; icon: React.ReactNode }[] = [
  { value: 'prompt', label: '提示词', icon: <MessageOutlined /> },
  { value: 'agent_ref', label: '子代理引用', icon: <ApiOutlined /> },
  { value: 'route', label: '路由', icon: <BranchesOutlined /> },
];

// ----------------------------------------------------------------
// SortableStepCard — one draggable step card
// ----------------------------------------------------------------

interface SortableStepCardProps {
  id: string;
  index: number;
  step: Step;
  availableAgents: AgentOption[];
  onUpdate: (index: number, patch: Partial<Step>) => void;
  onRemove: (index: number) => void;
  onTypeChange: (index: number, newType: StepType) => void;
  onCreateAgent?: () => void;
}

const SortableStepCard: React.FC<SortableStepCardProps> = ({
  id,
  index,
  step,
  availableAgents,
  onUpdate,
  onRemove,
  onTypeChange,
  onCreateAgent,
}) => {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id });

  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };

  return (
    <div ref={setNodeRef} style={style}>
      <Card
        size="small"
        title={
          <Space>
            <span
              {...attributes}
              {...listeners}
              style={{ cursor: 'grab', display: 'inline-flex', alignItems: 'center' }}
              title="拖拽排序"
            >
              <HolderOutlined style={{ fontSize: 14, color: '#999' }} />
            </span>
            <Text type="secondary">#{index + 1}</Text>
            <Select
              size="small"
              value={step.type}
              onChange={(val) => onTypeChange(index, val)}
              options={STEP_TYPE_OPTIONS.map((opt) => ({
                value: opt.value,
                label: (
                  <Space size={4}>
                    {opt.icon}
                    {opt.label}
                  </Space>
                ),
              }))}
              style={{ width: 160 }}
            />
            <Input
              size="small"
              placeholder="标签 (可选)"
              value={step.label || ''}
              onChange={(e) => onUpdate(index, { label: e.target.value })}
              style={{ width: 140 }}
            />
          </Space>
        }
        extra={
          <Button
            type="text"
            danger
            icon={<DeleteOutlined />}
            onClick={() => onRemove(index)}
            title="删除步骤"
          />
        }
      >
        {step.type === 'prompt' && (
          <TextArea
            rows={3}
            placeholder="提示词内容..."
            value={step.content || ''}
            onChange={(e) => onUpdate(index, { content: e.target.value })}
          />
        )}

        {step.type === 'agent_ref' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <Space.Compact style={{ width: '100%' }}>
              <Select
                placeholder="选择子代理"
                value={step.agent || undefined}
                onChange={(val) => onUpdate(index, { agent: val })}
                style={{ flex: 1 }}
                optionLabelProp="label"
              >
                {availableAgents.map((a) => (
                  <Select.Option key={a.name} value={a.name} label={a.name}>
                    <Space size={4}>
                      <span>{a.name}</span>
                      {a.tags?.map((t) => (
                        <Tag key={t} style={{ marginInlineEnd: 0, fontSize: 11, lineHeight: '18px', padding: '0 4px' }}>{t}</Tag>
                      ))}
                    </Space>
                  </Select.Option>
                ))}
              </Select>
              {onCreateAgent && (
                <Button icon={<PlusOutlined />} onClick={onCreateAgent}>
                  新建
                </Button>
              )}
            </Space.Compact>
            <TextArea
              rows={2}
              placeholder="任务描述（可选）— 告诉子代理需要做什么，留空则仅传递上一步结果"
              value={step.content || ''}
              onChange={(e) => onUpdate(index, { content: e.target.value })}
            />
          </div>
        )}

        {step.type === 'route' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <TextArea
              rows={2}
              placeholder="路由判断提示词..."
              value={step.prompt || ''}
              onChange={(e) => onUpdate(index, { prompt: e.target.value })}
            />
            <TextArea
              rows={2}
              placeholder="任务描述（可选）— 传递给路由目标子代理的任务说明"
              value={step.content || ''}
              onChange={(e) => onUpdate(index, { content: e.target.value })}
            />
            <RouterConfig
              branches={step.branches || {}}
              onChange={(branches) => onUpdate(index, { branches })}
              availableAgents={availableAgents}
            />
          </div>
        )}
      </Card>
    </div>
  );
};

// ----------------------------------------------------------------
// StepEditor — sortable list of step cards
// ----------------------------------------------------------------

export const StepEditor: React.FC<StepEditorProps> = ({
  steps,
  onChange,
  availableAgents,
  onCreateAgent,
}) => {
  // Stable sortable IDs keyed by index (reset on length change is fine for DnD)
  const itemIds = useMemo(() => steps.map((_, i) => `step-${i}`), [steps]);

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;
      if (!over || active.id === over.id) return;
      const oldIndex = itemIds.indexOf(active.id as string);
      const newIndex = itemIds.indexOf(over.id as string);
      if (oldIndex === -1 || newIndex === -1) return;
      const updated = [...steps];
      const [moved] = updated.splice(oldIndex, 1);
      updated.splice(newIndex, 0, moved);
      onChange(updated);
    },
    [steps, onChange, itemIds],
  );

  const updateStep = useCallback(
    (index: number, patch: Partial<Step>) => {
      const updated = steps.map((s, i) => (i === index ? { ...s, ...patch } : s));
      onChange(updated);
    },
    [steps, onChange],
  );

  const removeStep = useCallback(
    (index: number) => {
      onChange(steps.filter((_, i) => i !== index));
    },
    [steps, onChange],
  );

  const addStep = useCallback(() => {
    onChange([...steps, { type: 'prompt', label: '', content: '' }]);
  }, [steps, onChange]);

  const handleTypeChange = useCallback(
    (index: number, newType: StepType) => {
      const base: Step = { type: newType, label: steps[index].label };
      switch (newType) {
        case 'prompt':
          onChange(steps.map((s, i) => (i === index ? { ...base, content: '' } : s)));
          break;
        case 'agent_ref':
          onChange(steps.map((s, i) => (i === index ? { ...base, agent: '', content: '' } : s)));
          break;
        case 'route':
          onChange(
            steps.map((s, i) =>
              i === index ? { ...base, prompt: '', branches: { _default: '' } } : s,
            ),
          );
          break;
      }
    },
    [steps, onChange],
  );

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragEnd={handleDragEnd}
      >
        <SortableContext items={itemIds} strategy={verticalListSortingStrategy}>
          {steps.map((step, index) => (
            <SortableStepCard
              key={itemIds[index]}
              id={itemIds[index]}
              index={index}
              step={step}
              availableAgents={availableAgents}
              onUpdate={updateStep}
              onRemove={removeStep}
              onTypeChange={handleTypeChange}
              onCreateAgent={onCreateAgent}
            />
          ))}
        </SortableContext>
      </DndContext>

      <Button type="dashed" icon={<PlusOutlined />} onClick={addStep} block>
        添加步骤
      </Button>
    </div>
  );
};
