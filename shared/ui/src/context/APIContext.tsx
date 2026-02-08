// ================================================================
// API Context - Dependency injection for API adapter
// ================================================================

import React, { createContext, useContext, ReactNode } from 'react';
import { APIAdapter, createNoopAdapter } from '../services/adapter';

const APIContext = createContext<APIAdapter>(createNoopAdapter());

export interface APIProviderProps {
  adapter: APIAdapter;
  children: ReactNode;
}

/**
 * Provider component that injects the API adapter
 */
export const APIProvider: React.FC<APIProviderProps> = ({ adapter, children }) => {
  return (
    <APIContext.Provider value={adapter}>
      {children}
    </APIContext.Provider>
  );
};

/**
 * Hook to access the API adapter
 */
export const useAPI = (): APIAdapter => {
  const adapter = useContext(APIContext);
  if (!adapter) {
    throw new Error('useAPI must be used within an APIProvider');
  }
  return adapter;
};
