// ================================================================
// Status Indicator - Connection health indicator component
// ================================================================

import React from 'react';
import type { ConnectionHealthStatus } from '../context/ConnectionStatusContext';

export interface StatusIndicatorProps {
  status: ConnectionHealthStatus;
  label?: string;
  showLabel?: boolean;
  size?: 'sm' | 'md' | 'lg';
  onClick?: () => void;
  className?: string;
}

const sizeClasses = {
  sm: 'w-2 h-2',
  md: 'w-3 h-3',
  lg: 'w-4 h-4',
};

const statusColors: Record<ConnectionHealthStatus, string> = {
  healthy: 'bg-green-500',
  degraded: 'bg-yellow-500',
  error: 'bg-red-500',
  unknown: 'bg-gray-400',
};

const statusLabels: Record<ConnectionHealthStatus, string> = {
  healthy: '正常',
  degraded: '部分异常',
  error: '连接错误',
  unknown: '未知',
};

const pulseClasses: Record<ConnectionHealthStatus, string> = {
  healthy: '',
  degraded: 'animate-pulse',
  error: 'animate-pulse',
  unknown: '',
};

/**
 * Status indicator dot with optional label
 */
export const StatusIndicator: React.FC<StatusIndicatorProps> = ({
  status,
  label,
  showLabel = true,
  size = 'md',
  onClick,
  className = '',
}) => {
  const displayLabel = label || statusLabels[status];
  const isClickable = !!onClick;

  return (
    <div
      className={`
        inline-flex items-center gap-2
        ${isClickable ? 'cursor-pointer hover:opacity-80' : ''}
        ${className}
      `}
      onClick={onClick}
      role={isClickable ? 'button' : undefined}
      tabIndex={isClickable ? 0 : undefined}
      onKeyDown={isClickable ? (e) => e.key === 'Enter' && onClick?.() : undefined}
    >
      <span
        className={`
          rounded-full
          ${sizeClasses[size]}
          ${statusColors[status]}
          ${pulseClasses[status]}
        `}
        aria-hidden="true"
      />
      {showLabel && (
        <span className="text-sm text-muted-foreground">
          {displayLabel}
        </span>
      )}
    </div>
  );
};

// ================================================================
// Provider Status Badge - Shows individual provider status
// ================================================================

export interface ProviderStatusBadgeProps {
  name: string;
  available: boolean;
  error?: string;
  isRecovering?: boolean;
  onRecover?: () => void;
}

export const ProviderStatusBadge: React.FC<ProviderStatusBadgeProps> = ({
  name,
  available,
  error,
  isRecovering,
  onRecover,
}) => {
  const status: ConnectionHealthStatus = available ? 'healthy' : 'error';

  return (
    <div className="flex items-center gap-2 px-2 py-1 rounded bg-secondary/50">
      <StatusIndicator status={status} showLabel={false} size="sm" />
      <span className="text-sm font-medium">{name}</span>
      {error && (
        <span className="text-xs text-destructive truncate max-w-[150px]" title={error}>
          {error}
        </span>
      )}
      {!available && onRecover && (
        <button
          onClick={onRecover}
          disabled={isRecovering}
          className="text-xs text-primary hover:underline disabled:opacity-50"
        >
          {isRecovering ? '恢复中...' : '重连'}
        </button>
      )}
    </div>
  );
};

// ================================================================
// MCP Status Badge - Shows individual MCP server status
// ================================================================

export interface MCPStatusBadgeProps {
  name: string;
  status: 'running' | 'stopped' | 'error' | 'connected' | 'disconnected';
  error?: string;
  isReconnecting?: boolean;
  onReconnect?: () => void;
}

export const MCPStatusBadge: React.FC<MCPStatusBadgeProps> = ({
  name,
  status,
  error,
  isReconnecting,
  onReconnect,
}) => {
  const healthStatus: ConnectionHealthStatus = 
    status === 'running' || status === 'connected' ? 'healthy' :
    status === 'error' ? 'error' :
    status === 'stopped' || status === 'disconnected' ? 'unknown' : 'unknown';

  return (
    <div className="flex items-center gap-2 px-2 py-1 rounded bg-secondary/50">
      <StatusIndicator status={healthStatus} showLabel={false} size="sm" />
      <span className="text-sm font-medium">{name}</span>
      {error && (
        <span className="text-xs text-destructive truncate max-w-[150px]" title={error}>
          {error}
        </span>
      )}
      {status === 'error' && onReconnect && (
        <button
          onClick={onReconnect}
          disabled={isReconnecting}
          className="text-xs text-primary hover:underline disabled:opacity-50"
        >
          {isReconnecting ? '重连中...' : '重连'}
        </button>
      )}
    </div>
  );
};

export default StatusIndicator;
