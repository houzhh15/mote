import { useState, useEffect, useCallback, useImperativeHandle, forwardRef } from 'react';
import { message, Spin, Alert } from 'antd';
import { useAPI } from '../../context/APIContext';
import type { PolicyConfig, ApprovalRequest } from '../../types/policy';
import { PolicyOverviewCard } from './PolicyOverviewCard';
import { ListManagementCard } from './ListManagementCard';
import { DangerousOpsCard } from './DangerousOpsCard';
import { ParamRulesCard } from './ParamRulesCard';
import { PendingApprovalsCard } from './PendingApprovalsCard';
import { ScrubRulesCard } from './ScrubRulesCard';
import { BlockMessageCard } from './BlockMessageCard';

const defaultPolicy: PolicyConfig = {
  default_allow: true,
  require_approval: false,
  allowlist: [],
  blocklist: [],
  dangerous_ops: [],
  param_rules: {},
  scrub_rules: [],
  block_message_template: '',
  circuit_breaker_threshold: 3,
};

/** Handle exposed to parent for header button actions. */
export interface SecuritySettingsTabHandle {
  handleSave: () => Promise<void>;
  handleRefresh: () => Promise<void>;
  saving: boolean;
  loading: boolean;
}

export const SecuritySettingsTab = forwardRef<SecuritySettingsTabHandle>((_props, ref) => {
  const api = useAPI();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [policy, setPolicy] = useState<PolicyConfig>(defaultPolicy);
  const [pendingApprovals, setPendingApprovals] = useState<ApprovalRequest[]>([]);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      if (api.getPolicyConfig) {
        const cfg = await api.getPolicyConfig();
        setPolicy(cfg);
      }
      if (api.getApprovals) {
        const resp = await api.getApprovals();
        setPendingApprovals(resp.pending || []);
      }
    } catch (err) {
      console.error('Failed to load security settings:', err);
    } finally {
      setLoading(false);
    }
  }, [api]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const handlePolicyChange = (patch: Partial<PolicyConfig>) => {
    setPolicy((prev) => ({ ...prev, ...patch }));
  };

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      if (api.updatePolicyConfig) {
        await api.updatePolicyConfig(policy);
        message.success('安全策略已保存');
      } else {
        message.warning('Policy API 不可用');
      }
    } catch (err) {
      message.error('保存失败: ' + String(err));
    } finally {
      setSaving(false);
    }
  }, [api, policy]);

  const handleRefresh = useCallback(async () => {
    await loadData();
  }, [loadData]);

  // Expose save / refresh / state to parent for header buttons
  useImperativeHandle(ref, () => ({
    handleSave,
    handleRefresh,
    saving,
    loading,
  }), [handleSave, handleRefresh, saving, loading]);

  const handleApprove = async (id: string) => {
    try {
      if (api.respondApproval) {
        await api.respondApproval(id, true);
        message.success('已批准');
        loadData();
      }
    } catch (err) {
      message.error('操作失败: ' + String(err));
    }
  };

  const handleReject = async (id: string) => {
    try {
      if (api.respondApproval) {
        await api.respondApproval(id, false);
        message.success('已拒绝');
        loadData();
      }
    } catch (err) {
      message.error('操作失败: ' + String(err));
    }
  };

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: 48 }}>
        <Spin size="large" />
      </div>
    );
  }

  // Warn when default_allow=false and allowlist is empty — all tools will be denied
  const showEmptyAllowlistWarning = !policy.default_allow && policy.allowlist.length === 0;

  return (
    <div>
      {showEmptyAllowlistWarning && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message="白名单为空"
          description={'当前已关闭「默认允许工具执行」，但白名单为空。这意味着所有工具调用都会被拒绝。请添加允许使用的工具到白名单，或开启默认允许。'}
        />
      )}

      <PolicyOverviewCard
        policy={policy}
        pendingCount={pendingApprovals.length}
        onChange={handlePolicyChange}
      />

      <ListManagementCard
        policy={policy}
        onChange={handlePolicyChange}
      />

      <DangerousOpsCard
        policy={policy}
        onChange={handlePolicyChange}
      />

      <ParamRulesCard
        policy={policy}
        onChange={handlePolicyChange}
      />

      <ScrubRulesCard
        policy={policy}
        onChange={handlePolicyChange}
      />

      <BlockMessageCard
        policy={policy}
        onChange={handlePolicyChange}
      />

      <PendingApprovalsCard
        pending={pendingApprovals}
        onApprove={handleApprove}
        onReject={handleReject}
      />
    </div>
  );
});

SecuritySettingsTab.displayName = 'SecuritySettingsTab';
