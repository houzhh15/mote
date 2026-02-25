/** Policy configuration types matching Go backend structures. */

export interface ScrubRule {
  name: string;
  pattern: string;
  replacement: string;
  enabled: boolean;
}

export interface PolicyConfig {
  default_allow: boolean;
  require_approval: boolean;
  allowlist: string[];
  blocklist: string[];
  dangerous_ops: DangerousOpRule[];
  param_rules: Record<string, ParamRule>;
  scrub_rules: ScrubRule[];
  block_message_template: string;
  circuit_breaker_threshold: number;
}

export interface DangerousOpRule {
  tool: string;
  pattern: string;
  severity: 'low' | 'medium' | 'high' | 'critical';
  action: 'block' | 'approve' | 'warn';
  message: string;
  enabled?: boolean;
}

export interface ParamRule {
  max_length?: number;
  pattern?: string;
  forbidden?: string[];
  path_prefix?: string[];
  enabled?: boolean;
}

export interface PolicyStatus {
  default_allow: boolean;
  require_approval: boolean;
  blocklist_count: number;
  allowlist_count: number;
  dangerous_rules_count: number;
  param_rules_count: number;
  message?: string;
}

export interface PolicyCheckRequest {
  tool: string;
  arguments: string;
  session_id?: string;
}

export interface PolicyCheckResponse {
  tool: string;
  allowed: boolean;
  require_approval: boolean;
  blocked: boolean;
  reason?: string;
  warnings?: string[];
}

export interface ApprovalRequest {
  id: string;
  tool_name: string;
  arguments: string;
  reason: string;
  session_id: string;
  agent_id: string;
  created_at: string;
  expires_at: string;
}

export interface ApprovalListResponse {
  pending: ApprovalRequest[];
  count: number;
}
