// ================================================================
// VllmIcon - vLLM branding icon component
// ================================================================

import React from 'react';

interface VllmIconProps {
  size?: number;
  style?: React.CSSProperties;
  className?: string;
  color?: string;
}

/**
 * vLLM icon – a stylised "V" glyph representing vLLM.
 * Designed as a simple recognisable SVG that works at small sizes.
 */
export const VllmIcon: React.FC<VllmIconProps> = ({
  size = 16,
  style,
  className,
  color = 'currentColor',
}) => {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      style={{ display: 'inline-block', verticalAlign: 'middle', ...style }}
      className={className}
    >
      {/* Rounded rect background */}
      <rect x="3" y="3" width="18" height="18" rx="4" stroke={color} strokeWidth="1.8" fill="none" />
      {/* Stylised "V" */}
      <path
        d="M7 8 L12 17 L17 8"
        stroke={color}
        strokeWidth="2.2"
        strokeLinecap="round"
        strokeLinejoin="round"
        fill="none"
      />
    </svg>
  );
};

export default VllmIcon;
