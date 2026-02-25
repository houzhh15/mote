// ================================================================
// GlmIcon - GLM (智谱AI) branding icon component
// ================================================================

import React from 'react';

interface GlmIconProps {
  size?: number;
  style?: React.CSSProperties;
  className?: string;
  color?: string;
}

/**
 * GLM icon - stylised "Z" glyph representing 智谱 (Zhipu).
 * Designed as a simple recognisable SVG that works at small sizes.
 */
export const GlmIcon: React.FC<GlmIconProps> = ({
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
      {/* Stylised brain/AI chip icon */}
      <rect x="3" y="3" width="18" height="18" rx="4" stroke={color} strokeWidth="1.8" fill="none" />
      <text
        x="12"
        y="17"
        textAnchor="middle"
        fontFamily="Arial, sans-serif"
        fontWeight="bold"
        fontSize="13"
        fill={color}
      >
        Z
      </text>
    </svg>
  );
};

export default GlmIcon;
