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
  const bannerClasses = isWarning
    ? 'bg-yellow-50 border-yellow-200 text-yellow-800 dark:bg-yellow-900/20 dark:border-yellow-800 dark:text-yellow-200'
    : 'bg-red-50 border-red-200 text-red-800 dark:bg-red-900/20 dark:border-red-800 dark:text-red-200';

  return (
    <div
      className={`
        flex items-center justify-between gap-4
        px-4 py-3 rounded-lg border
        ${bannerClasses}
      `}
      role="alert"
    >
      {/* Error icon */}
      <div className="flex items-center gap-3">
        <svg
          className="w-5 h-5 flex-shrink-0"
          fill="currentColor"
          viewBox="0 0 20 20"
        >
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
        <div className="flex flex-col">
          <span className="font-medium">
            {sourceLabels[source]} ({name}): {friendlyMessage}
          </span>
          {detail.message !== friendlyMessage && (
            <span className="text-sm opacity-80">{detail.message}</span>
          )}
          {actions.wait && detail.retry_after && detail.retry_after > 0 && (
            <span className="text-sm">
              请等待 {detail.retry_after} 秒后重试
            </span>
          )}
        </div>
      </div>

      {/* Action buttons */}
      <div className="flex items-center gap-2">
        {/* Retry button */}
        {actions.retry && detail.retryable && onRetry && (
          <button
            onClick={onRetry}
            disabled={isRecovering}
            className={`
              px-3 py-1 text-sm font-medium rounded
              ${isWarning
                ? 'bg-yellow-200 hover:bg-yellow-300 dark:bg-yellow-800 dark:hover:bg-yellow-700'
                : 'bg-red-200 hover:bg-red-300 dark:bg-red-800 dark:hover:bg-red-700'
              }
              disabled:opacity-50 disabled:cursor-not-allowed
            `}
          >
            {isRecovering ? '重试中...' : '重试'}
          </button>
        )}

        {/* Reauth button */}
        {actions.reauth && onReauth && (
          <button
            onClick={onReauth}
            disabled={isRecovering}
            className={`
              px-3 py-1 text-sm font-medium rounded
              ${isWarning
                ? 'bg-yellow-200 hover:bg-yellow-300 dark:bg-yellow-800 dark:hover:bg-yellow-700'
                : 'bg-red-200 hover:bg-red-300 dark:bg-red-800 dark:hover:bg-red-700'
              }
              disabled:opacity-50 disabled:cursor-not-allowed
            `}
          >
            {isRecovering ? '认证中...' : '重新认证'}
          </button>
        )}

        {/* Dismiss button */}
        {onDismiss && (
          <button
            onClick={onDismiss}
            className="p-1 rounded hover:bg-black/10 dark:hover:bg-white/10"
            aria-label="关闭"
          >
            <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
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
  return (
    <div className="flex items-center gap-2 px-3 py-2 text-sm bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300 rounded">
      <svg className="w-4 h-4 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
        <path
          fillRule="evenodd"
          d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
          clipRule="evenodd"
        />
      </svg>
      <span className="flex-1">{message}</span>
      {retryable && onRetry && (
        <button
          onClick={onRetry}
          className="text-xs font-medium text-red-600 hover:underline dark:text-red-400"
        >
          重试
        </button>
      )}
      {onDismiss && (
        <button onClick={onDismiss} className="p-0.5 hover:opacity-70" aria-label="关闭">
          <svg className="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
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
