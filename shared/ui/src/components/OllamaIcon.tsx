// ================================================================
// OllamaIcon - Ollama branding icon component
// ================================================================

import React from 'react';
import ollamaLogo from '../assets/ollama.png';

interface OllamaIconProps {
  size?: number;
  style?: React.CSSProperties;
  className?: string;
}

/**
 * Ollama icon - using official Ollama logo
 */
export const OllamaIcon: React.FC<OllamaIconProps> = ({ 
  size = 16, 
  style,
  className 
}) => {
  return (
    <img
      src={ollamaLogo}
      alt="Ollama"
      width={size}
      height={size}
      style={{ display: 'inline-block', verticalAlign: 'middle', objectFit: 'contain', ...style }}
      className={className}
    />
  );
};

export default OllamaIcon;
