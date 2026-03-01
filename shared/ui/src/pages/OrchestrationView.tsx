import React, { useCallback, useMemo } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  Handle,
  Position,
  type Node,
  type Edge,
  type NodeProps,
  MarkerType,
  useNodesState,
  useEdgesState,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import dagre from 'dagre';
import { Tooltip, Typography } from 'antd';
import {
  MessageOutlined,
  ApiOutlined,
  BranchesOutlined,
} from '@ant-design/icons';
import type { AgentConfig, Step } from '../types';

const { Text } = Typography;

// ================================================================
// Custom Node Components
// ================================================================

interface PromptNodeData extends Record<string, unknown> {
  label: string;
  content: string;
  agentName: string;
}

const PromptNode: React.FC<NodeProps<Node<PromptNodeData>>> = ({ data }) => (
  <Tooltip title={data.content || '(空 prompt)'}>
    <div
      style={{
        background: '#e6f4ff',
        border: '2px solid #1677ff',
        borderRadius: 8,
        padding: '8px 12px',
        minWidth: 120,
        textAlign: 'center',
      }}
    >
      <Handle type="target" position={Position.Top} />
      <MessageOutlined style={{ color: '#1677ff', marginRight: 4 }} />
      <Text strong style={{ fontSize: 12 }}>
        {data.label || 'Prompt'}
      </Text>
      <div style={{ fontSize: 11, color: '#666', marginTop: 2 }}>
        {(data.content || '').slice(0, 30)}
        {(data.content || '').length > 30 ? '...' : ''}
      </div>
      <Handle type="source" position={Position.Bottom} />
    </div>
  </Tooltip>
);

interface AgentNodeData extends Record<string, unknown> {
  label: string;
  agentName: string;
  stepCount: number;
}

const AgentNode: React.FC<NodeProps<Node<AgentNodeData>>> = ({ data }) => (
  <Tooltip title={`代理 ${data.agentName} — ${data.stepCount} 个步骤`}>
    <div
      style={{
        background: '#f6ffed',
        border: '2px solid #52c41a',
        borderRadius: 4,
        padding: '8px 12px',
        minWidth: 120,
        textAlign: 'center',
      }}
    >
      <Handle type="target" position={Position.Top} />
      <ApiOutlined style={{ color: '#52c41a', marginRight: 4 }} />
      <Text strong style={{ fontSize: 12 }}>
        {data.label}
      </Text>
      <div style={{ fontSize: 11, color: '#666', marginTop: 2 }}>
        {data.stepCount} 步骤
      </div>
      <Handle type="source" position={Position.Bottom} />
    </div>
  </Tooltip>
);

interface RouteNodeData extends Record<string, unknown> {
  label: string;
  agentName: string;
  branchCount: number;
  prompt: string;
}

const RouteNode: React.FC<NodeProps<Node<RouteNodeData>>> = ({ data }) => (
  <Tooltip title={`路由判断: ${data.prompt || '(无 prompt)'}\n分支数: ${data.branchCount}`}>
    <div
      style={{
        background: '#fff7e6',
        border: '2px solid #fa8c16',
        borderRadius: 4,
        padding: '8px 12px',
        minWidth: 100,
        textAlign: 'center',
        transform: 'rotate(0deg)',
      }}
    >
      <Handle type="target" position={Position.Top} />
      <BranchesOutlined style={{ color: '#fa8c16', marginRight: 4 }} />
      <Text strong style={{ fontSize: 12 }}>
        {data.label || 'Route'}
      </Text>
      <div style={{ fontSize: 11, color: '#666', marginTop: 2 }}>
        {data.branchCount} 分支
      </div>
      <Handle type="source" position={Position.Bottom} />
    </div>
  </Tooltip>
);

const nodeTypes = {
  promptNode: PromptNode,
  agentNode: AgentNode,
  routeNode: RouteNode,
};

// ================================================================
// Layout Helper (dagre)
// ================================================================

function applyDagreLayout(nodes: Node[], edges: Edge[]): Node[] {
  const g = new dagre.graphlib.Graph();
  g.setDefaultEdgeLabel(() => ({}));
  g.setGraph({ rankdir: 'TB', ranksep: 60, nodesep: 40, marginx: 20, marginy: 20 });

  nodes.forEach((n) => {
    g.setNode(n.id, { width: 160, height: 70 });
  });
  edges.forEach((e) => {
    g.setEdge(e.source, e.target);
  });

  dagre.layout(g);

  return nodes.map((n) => {
    const pos = g.node(n.id);
    return {
      ...n,
      position: { x: pos.x - 80, y: pos.y - 35 },
    };
  });
}

// ================================================================
// Graph generation from AgentConfig
// ================================================================

function buildGraph(
  agents: Record<string, AgentConfig>,
): { nodes: Node[]; edges: Edge[] } {
  const nodes: Node[] = [];
  const edges: Edge[] = [];
  let nodeIndex = 0;

  // Create agent-level root nodes for agents with steps
  const agentNodeIds: Record<string, string> = {};

  for (const [agentName, agentConfig] of Object.entries(agents)) {
    if (!agentConfig.steps || agentConfig.steps.length === 0) continue;

    const rootId = `agent-${agentName}`;
    agentNodeIds[agentName] = rootId;
    nodes.push({
      id: rootId,
      type: 'agentNode',
      position: { x: 0, y: 0 },
      data: {
        label: agentName,
        agentName,
        stepCount: agentConfig.steps.length,
      },
    });
    nodeIndex++;

    // Create step nodes
    let prevNodeId = rootId;
    agentConfig.steps.forEach((step: Step, stepIdx: number) => {
      const stepId = `${agentName}-step-${stepIdx}`;

      if (step.type === 'prompt') {
        nodes.push({
          id: stepId,
          type: 'promptNode',
          position: { x: 0, y: 0 },
          data: {
            label: step.label || `Prompt ${stepIdx + 1}`,
            content: step.content || '',
            agentName,
          },
        });
      } else if (step.type === 'agent_ref') {
        nodes.push({
          id: stepId,
          type: 'agentNode',
          position: { x: 0, y: 0 },
          data: {
            label: step.agent || '(unknown)',
            agentName: step.agent || '',
            stepCount: agents[step.agent || '']?.steps?.length || 0,
          },
        });
      } else if (step.type === 'route') {
        const branchCount = step.branches ? Object.keys(step.branches).length : 0;
        nodes.push({
          id: stepId,
          type: 'routeNode',
          position: { x: 0, y: 0 },
          data: {
            label: step.label || `Route ${stepIdx + 1}`,
            agentName,
            branchCount,
            prompt: step.prompt || '',
          },
        });
      }

      nodeIndex++;

      // Sequential edge from previous step
      edges.push({
        id: `e-${prevNodeId}-${stepId}`,
        source: prevNodeId,
        target: stepId,
        animated: false,
        style: { stroke: '#999' },
      });

      // If route, add branch edges
      if (step.type === 'route' && step.branches) {
        for (const [match, target] of Object.entries(step.branches)) {
          const targetNodeId = agentNodeIds[target] || `agent-${target}`;
          const isSelf = target === agentName;

          // Ensure target node exists (might be a simple agent without steps)
          if (!nodes.find((n) => n.id === targetNodeId) && !isSelf) {
            nodes.push({
              id: targetNodeId,
              type: 'agentNode',
              position: { x: 0, y: 0 },
              data: {
                label: target,
                agentName: target,
                stepCount: agents[target]?.steps?.length || 0,
              },
            });
            agentNodeIds[target] = targetNodeId;
            nodeIndex++;
          }

          edges.push({
            id: `e-route-${stepId}-${match}-${target}`,
            source: stepId,
            target: isSelf ? rootId : targetNodeId,
            label: match === '_default' ? 'default' : match,
            animated: true,
            style: {
              stroke: isSelf ? '#f5222d' : '#fa8c16',
              strokeDasharray: '5,5',
            },
            markerEnd: { type: MarkerType.ArrowClosed },
            ...(isSelf
              ? {
                  type: 'smoothstep' as const,
                  data: { label: `max: ${agentConfig.max_recursion ?? '∞'}` },
                }
              : {}),
          });
        }
      }

      // If agent_ref, add edge to target agent
      if (step.type === 'agent_ref' && step.agent) {
        const targetNodeId = agentNodeIds[step.agent] || `agent-${step.agent}`;
        if (!nodes.find((n) => n.id === targetNodeId)) {
          nodes.push({
            id: targetNodeId,
            type: 'agentNode',
            position: { x: 0, y: 0 },
            data: {
              label: step.agent,
              agentName: step.agent,
              stepCount: agents[step.agent]?.steps?.length || 0,
            },
          });
          agentNodeIds[step.agent] = targetNodeId;
          nodeIndex++;
        }
      }

      prevNodeId = stepId;
    });
  }

  const layoutNodes = applyDagreLayout(nodes, edges);
  return { nodes: layoutNodes, edges };
}

// ================================================================
// OrchestrationView Component
// ================================================================

interface OrchestrationViewProps {
  agents: Record<string, AgentConfig>;
  onEditAgent?: (name: string, config: AgentConfig) => void;
}

const OrchestrationView: React.FC<OrchestrationViewProps> = ({
  agents,
  onEditAgent,
}) => {
  const { nodes: initialNodes, edges: initialEdges } = useMemo(
    () => buildGraph(agents),
    [agents],
  );

  const [nodes, , onNodesChange] = useNodesState(initialNodes);
  const [edges, , onEdgesChange] = useEdgesState(initialEdges);

  const onNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      const agentName = (node.data as Record<string, unknown>).agentName as string;
      if (agentName && agents[agentName] && onEditAgent) {
        onEditAgent(agentName, agents[agentName]);
      }
    },
    [agents, onEditAgent],
  );

  const hasStructuredAgents = Object.values(agents).some(
    (a) => a.steps && a.steps.length > 0,
  );

  if (!hasStructuredAgents) {
    return (
      <div style={{ textAlign: 'center', padding: 40, color: '#999' }}>
        <BranchesOutlined style={{ fontSize: 48, marginBottom: 16 }} />
        <div>暂无结构化编排的代理。</div>
        <div style={{ fontSize: 12, marginTop: 4 }}>
          在代理编辑中切换到「结构化编排」模式以开始配置。
        </div>
      </div>
    );
  }

  return (
    <div style={{ width: '100%', height: 480 }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onNodeClick={onNodeClick}
        fitView
        proOptions={{ hideAttribution: true }}
      >
        <Background />
        <Controls />
      </ReactFlow>
    </div>
  );
};

export default OrchestrationView;
