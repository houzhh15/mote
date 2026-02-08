// ================================================================
// Connection Status Context - Monitor provider and MCP connection health
// ================================================================

import React, {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  ReactNode,
  useMemo,
} from 'react';
import { useAPI } from './APIContext';
import type { ProviderStatus, MCPServer, ErrorDetail } from '../types';

// ================================================================
// Types
// ================================================================

export type ConnectionHealthStatus = 'healthy' | 'degraded' | 'error' | 'unknown';

export interface ProviderConnectionStatus extends ProviderStatus {
  lastChecked?: Date;
  isRecovering?: boolean;
}

export interface MCPConnectionStatus {
  name: string;
  status: MCPServer['status'];
  error?: string;
  lastChecked?: Date;
  isReconnecting?: boolean;
}

export interface ConnectionStatus {
  // Overall status
  overallHealth: ConnectionHealthStatus;
  
  // Provider statuses
  providers: ProviderConnectionStatus[];
  
  // MCP server statuses
  mcpServers: MCPConnectionStatus[];
  
  // Stream connection
  streamConnected: boolean;
  
  // Current active error (for banner display)
  activeError?: {
    source: 'provider' | 'mcp' | 'stream';
    name: string;
    detail: ErrorDetail;
  };
  
  // Last update timestamp
  lastUpdated?: Date;
}

export interface ConnectionStatusContextValue {
  status: ConnectionStatus;
  
  // Actions
  refreshStatus: () => Promise<void>;
  recoverProvider: (providerName: string, action?: 'reconnect' | 'reauth') => Promise<boolean>;
  reconnectMCP: (serverName: string) => Promise<boolean>;
  dismissError: () => void;
  
  // Stream error handler (called by ChatManager)
  handleStreamError: (error: ErrorDetail, provider?: string) => void;
  clearStreamError: () => void;
  
  // Loading state
  isLoading: boolean;
}

// ================================================================
// Context
// ================================================================

const defaultStatus: ConnectionStatus = {
  overallHealth: 'unknown',
  providers: [],
  mcpServers: [],
  streamConnected: true,
};

const ConnectionStatusContext = createContext<ConnectionStatusContextValue | null>(null);

// ================================================================
// Provider Component
// ================================================================

export interface ConnectionStatusProviderProps {
  children: ReactNode;
  pollInterval?: number; // Polling interval in ms (default: 30000)
  enablePolling?: boolean; // Enable/disable background polling
}

export const ConnectionStatusProvider: React.FC<ConnectionStatusProviderProps> = ({
  children,
  pollInterval = 30000,
  enablePolling = true,
}) => {
  const api = useAPI();
  const [status, setStatus] = useState<ConnectionStatus>(defaultStatus);
  const [isLoading, setIsLoading] = useState(false);

  // Fetch provider status
  const fetchProviderStatus = useCallback(async (): Promise<ProviderConnectionStatus[]> => {
    try {
      const modelsResponse = await api.getModels();
      const now = new Date();
      return (modelsResponse.providers || []).map(p => ({
        ...p,
        lastChecked: now,
      }));
    } catch (err) {
      console.error('Failed to fetch provider status:', err);
      return [];
    }
  }, [api]);

  // Fetch MCP status
  const fetchMCPStatus = useCallback(async (): Promise<MCPConnectionStatus[]> => {
    try {
      const servers = await api.getMCPServers();
      const now = new Date();
      return servers.map(s => ({
        name: s.name,
        status: s.status,
        error: s.error,
        lastChecked: now,
      }));
    } catch (err) {
      console.error('Failed to fetch MCP status:', err);
      return [];
    }
  }, [api]);

  // Calculate overall health
  const calculateOverallHealth = useCallback(
    (providers: ProviderConnectionStatus[], mcpServers: MCPConnectionStatus[]): ConnectionHealthStatus => {
      // Check if any provider has an error
      const hasProviderError = providers.some(p => !p.available || p.error);
      
      // Check if any MCP server has an error
      const hasMCPError = mcpServers.some(s => s.status === 'error');
      
      // All providers unavailable = error
      const allProvidersUnavailable = providers.length > 0 && providers.every(p => !p.available);
      
      if (allProvidersUnavailable) {
        return 'error';
      }
      
      if (hasProviderError || hasMCPError) {
        return 'degraded';
      }
      
      if (providers.length === 0) {
        return 'unknown';
      }
      
      return 'healthy';
    },
    []
  );

  // Refresh all status
  const refreshStatus = useCallback(async () => {
    setIsLoading(true);
    try {
      const [providers, mcpServers] = await Promise.all([
        fetchProviderStatus(),
        fetchMCPStatus(),
      ]);

      setStatus(prev => ({
        ...prev,
        providers,
        mcpServers,
        overallHealth: calculateOverallHealth(providers, mcpServers),
        lastUpdated: new Date(),
      }));
    } catch (err) {
      console.error('Failed to refresh connection status:', err);
    } finally {
      setIsLoading(false);
    }
  }, [fetchProviderStatus, fetchMCPStatus, calculateOverallHealth]);

  // Initial fetch and polling
  useEffect(() => {
    refreshStatus();

    if (!enablePolling) return;

    const interval = setInterval(refreshStatus, pollInterval);
    return () => clearInterval(interval);
  }, [refreshStatus, pollInterval, enablePolling]);

  // Recover provider
  const recoverProvider = useCallback(async (providerName: string, action: 'reconnect' | 'reauth' = 'reconnect'): Promise<boolean> => {
    // Mark as recovering
    setStatus(prev => ({
      ...prev,
      providers: prev.providers.map(p =>
        p.name === providerName ? { ...p, isRecovering: true } : p
      ),
    }));

    try {
      // Call recovery API - need to implement in adapter
      const response = await fetch(`/api/v1/providers/${providerName}/recover`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action }),
      });

      const result = await response.json();
      
      // Refresh status after recovery
      await refreshStatus();
      
      return result.success === true;
    } catch (err) {
      console.error(`Failed to recover provider ${providerName}:`, err);
      
      // Clear recovering state
      setStatus(prev => ({
        ...prev,
        providers: prev.providers.map(p =>
          p.name === providerName ? { ...p, isRecovering: false } : p
        ),
      }));
      
      return false;
    }
  }, [refreshStatus]);

  // Reconnect MCP server
  const reconnectMCP = useCallback(async (serverName: string): Promise<boolean> => {
    // Mark as reconnecting
    setStatus(prev => ({
      ...prev,
      mcpServers: prev.mcpServers.map(s =>
        s.name === serverName ? { ...s, isReconnecting: true } : s
      ),
    }));

    try {
      // Stop and start the server
      await api.stopMCPServer(serverName);
      await new Promise(resolve => setTimeout(resolve, 500)); // Brief delay
      await api.startMCPServer(serverName);
      
      // Refresh status
      await refreshStatus();
      
      return true;
    } catch (err) {
      console.error(`Failed to reconnect MCP server ${serverName}:`, err);
      
      // Clear reconnecting state
      setStatus(prev => ({
        ...prev,
        mcpServers: prev.mcpServers.map(s =>
          s.name === serverName ? { ...s, isReconnecting: false } : s
        ),
      }));
      
      return false;
    }
  }, [api, refreshStatus]);

  // Handle stream error
  const handleStreamError = useCallback((error: ErrorDetail, provider?: string) => {
    setStatus(prev => ({
      ...prev,
      streamConnected: false,
      activeError: {
        source: 'stream',
        name: provider || 'unknown',
        detail: error,
      },
    }));
  }, []);

  // Clear stream error
  const clearStreamError = useCallback(() => {
    setStatus(prev => ({
      ...prev,
      streamConnected: true,
      activeError: prev.activeError?.source === 'stream' ? undefined : prev.activeError,
    }));
  }, []);

  // Dismiss active error
  const dismissError = useCallback(() => {
    setStatus(prev => ({
      ...prev,
      activeError: undefined,
    }));
  }, []);

  // Memoize context value
  const contextValue = useMemo<ConnectionStatusContextValue>(
    () => ({
      status,
      refreshStatus,
      recoverProvider,
      reconnectMCP,
      dismissError,
      handleStreamError,
      clearStreamError,
      isLoading,
    }),
    [status, refreshStatus, recoverProvider, reconnectMCP, dismissError, handleStreamError, clearStreamError, isLoading]
  );

  return (
    <ConnectionStatusContext.Provider value={contextValue}>
      {children}
    </ConnectionStatusContext.Provider>
  );
};

// ================================================================
// Hook
// ================================================================

export const useConnectionStatus = (): ConnectionStatusContextValue => {
  const context = useContext(ConnectionStatusContext);
  if (!context) {
    throw new Error('useConnectionStatus must be used within a ConnectionStatusProvider');
  }
  return context;
};

// ================================================================
// Utility Hooks
// ================================================================

/**
 * Hook to get a specific provider's status
 */
export const useProviderStatus = (providerName: string): ProviderConnectionStatus | undefined => {
  const { status } = useConnectionStatus();
  return status.providers.find(p => p.name === providerName);
};

/**
 * Hook to get a specific MCP server's status
 */
export const useMCPStatus = (serverName: string): MCPConnectionStatus | undefined => {
  const { status } = useConnectionStatus();
  return status.mcpServers.find(s => s.name === serverName);
};

/**
 * Hook to check if any connection has issues
 */
export const useHasConnectionIssues = (): boolean => {
  const { status } = useConnectionStatus();
  return status.overallHealth !== 'healthy' && status.overallHealth !== 'unknown';
};
