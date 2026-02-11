// ================================================================
// Error Banner - Display connection errors with recovery actions
// ================================================================

import React from 'react';
import type { ErrorDetail } from '../types';

export interface ErrorBannerProps {
  source: 'provider' | 'mcp' | 'stream';
  name: string;
  detail: ErrorDetail;
  onDismiss?: () => void;
  onRetry?: () => void;
  onReauth?: () => void;
  isRecovering?: boolean;
}

// Error code to user-friendly message mapping
const errorMessages: Record<string, string> = {
  AUTH_FAILED: '认证失败，请检查授权',
  TOKEN_EXPIRED: 'Token 已过期，需要重新认证',
  RATE_LIMITED: '请求过于频繁，已触发限速',
  QUOTA_EXCEEDED: '配额已用尽',
  SERVICE_UNAVAILABLE: '服务暂时不可用',
  MODEL_NOT_FOUND: '模型不存在或不可用',
  NETWORK_ERROR: '网络连接错误',
  INVALID_REQUEST: '请求错误',
  TIMEOUT: '请求超时',
  UNKNOWN: '未知错误',
};

// Error code to suggested action mapping
const errorActions: Record<string, { retry: boolean; reauth: boolean; wait: boolean }> = {
  AUTH_FAILED: { retry: false, reauth: true, wait: false },
  TOKEN_EXPIRED: { retry: false, reauth: true, wait: false },
  RATE_LIMITED: { retry: false, reauth: false, wait: true },
  QUOTA_EXCEEDED: { retry: false, reauth: false, wait: false },
  SERVICE_UNAVAILABLE: { retry: true, reauth: false, wait: false },
  MODEL_NOT_FOUND: { retry: false, reauth: false, wait: false },
  NETWORK_ERROR: { retry: true, reauth: false, wait: false },
  INVALID_REQUEST: { retry: false, reauth: false, wait: false },
  TIMEOUT: { retry: true, reauth: false, wait: false },
  UNKNOWN: { retry: true, reauth: false, wait: false },
};

// Source label mapping
const sourceLabels: Record<string, string> = {
  provider: 'Provider',
  mcp: 'MCP 服务器',
  stream: '流式连接',
};

/**
 * Error banner component for displaying connection errors
 */
export const ErrorBanner: React.FC<ErrorBannerProps> = ({
  source,
  name,
  detail,
  onDismiss,
  onRetry,
  onReauth,
  isRecovering,
}) => {
  const friendlyMessage = errorMessages[detail.code] || detail.message;
  const actions = errorActions[detail.code] || { retry: true, reauth: false, wait: false };

  // Determine banner variant based on error severity
  const isWarning = detail.code === 'RATE_LIMITED' || detail.code === 'QUOTA_EXCEEDED';
  
  // Inline styles for banner (no Tailwind dependency)
  const bannerStyle: React.CSSProperties = {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: 16,
    padding: '12px 16px',
    borderRadius: 8,
    border: '1px solid',
    backgroundColor: isWarning ? '#fefce8' : '#fef2f2',
    borderColor: isWarning ? '#fde047' : '#fecaca',
    color: isWarning ? '#854d0e' : '#991b1b',
  };

  const iconStyle: React.CSSProperties = {
    width: 20,
    height: 20,
    flexShrink: 0,
  };

  const buttonStyle: React.CSSProperties = {
    padding: '4px 12px',
    fontSize: 14,
    fontWeight: 500,
    borderRadius: 4,
    border: 'none',
    cursor: 'pointer',
    backgroundColor: isWarning ? '#fde047' : '#fecaca',
    color: isWarning ? '#854d0e' : '#991b1b',
  };

  const dismissButtonStyle: React.CSSProperties = {
    padding: 4,
    borderRadius: 4,
    border: 'none',
    cursor: 'pointer',
    backgroundColor: 'transparent',
    color: 'inherit',
  };

  return (
    <div style={bannerStyle} role="alert">
      {/* Error icon and content */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <svg style={iconStyle} fill="currentColor" viewBox="0 0 20 20">
          {isWarning ? (
            <path
              fillRule="evenodd"
              d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z"
              clipRule="evenodd"
            />
          ) : (
            <path
              fillRule="evenodd"
              d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
              clipRule="evenodd"
            />
          )}
        </svg>

        {/* Error content */}
        <div style={{ display: 'flex', flexDirection: 'column' }}>
          <span style={{ fontWeight: 500 }}>
            {sourceLabels[source]} ({name}): {friendlyMessage}
          </span>
          {detail.message !== friendlyMessage && (
            <span style={{ fontSize: 14, opacity: 0.8 }}>{detail.message}</span>
          )}
          {actions.wait && detail.retry_after && detail.retry_after > 0 && (
            <span style={{ fontSize: 14 }}>
              请等待 {detail.retry_after} 秒后重试
            </span>
          )}
        </div>
      </div>

      {/* Action buttons */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        {/* Retry button */}
        {actions.retry && detail.retryable && onRetry && (
          <button
            onClick={onRetry}
            disabled={isRecovering}
            style={{
              ...buttonStyle,
              opacity: isRecovering ? 0.5 : 1,
              cursor: isRecovering ? 'not-allowed' : 'pointer',
            }}
          >
            {isRecovering ? '重试中...' : '重试'}
          </button>
        )}

        {/* Reauth button */}
        {actions.reauth && onReauth && (
          <button
            onClick={onReauth}
            disabled={isRecovering}
            style={{
              ...buttonStyle,
              opacity: isRecovering ? 0.5 : 1,
              cursor: isRecovering ? 'not-allowed' : 'pointer',
            }}
          >
            {isRecovering ? '认证中...' : '重新认证'}
          </button>
        )}

        {/* Dismiss button */}
        {onDismiss && (
          <button onClick={onDismiss} style={dismissButtonStyle} aria-label="关闭">
            <svg style={{ width: 16, height: 16 }} fill="currentColor" viewBox="0 0 20 20">
              <path
                fillRule="evenodd"
                d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
                clipRule="evenodd"
              />
            </svg>
          </button>
        )}
      </div>
    </div>
  );
};

// ================================================================
// Compact Error Banner - Smaller version for inline display
// ================================================================

export interface CompactErrorBannerProps {
  message: string;
  retryable?: boolean;
  onRetry?: () => void;
  onDismiss?: () => void;
}

export const CompactErrorBanner: React.FC<CompactErrorBannerProps> = ({
  message,
  retryable,
  onRetry,
  onDismiss,
}) => {
  const containerStyle: React.CSSProperties = {
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    padding: '8px 12px',
    fontSize: 14,
    backgroundColor: '#fef2f2',
    color: '#b91c1c',
    borderRadius: 4,
  };

  return (
    <div style={containerStyle}>
      <svg style={{ width: 16, height: 16, flexShrink: 0 }} fill="currentColor" viewBox="0 0 20 20">
        <path
          fillRule="evenodd"
          d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
          clipRule="evenodd"
        />
      </svg>
      <span style={{ flex: 1 }}>{message}</span>
      {retryable && onRetry && (
        <button
          onClick={onRetry}
          style={{
            fontSize: 12,
            fontWeight: 500,
            color: '#dc2626',
            background: 'none',
            border: 'none',
            cursor: 'pointer',
            textDecoration: 'underline',
          }}
        >
          重试
        </button>
      )}
      {onDismiss && (
        <button 
          onClick={onDismiss} 
          style={{ 
            padding: 2, 
            background: 'none', 
            border: 'none', 
            cursor: 'pointer',
            color: 'inherit',
          }} 
          aria-label="关闭"
        >
          <svg style={{ width: 12, height: 12 }} fill="currentColor" viewBox="0 0 20 20">
            <path
              fillRule="evenodd"
              d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
              clipRule="evenodd"
            />
          </svg>
        </button>
      )}
    </div>
  );
};

export default ErrorBanner;
