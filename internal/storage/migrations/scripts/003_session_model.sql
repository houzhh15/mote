-- 003_session_model.sql: 为 Session 添加 model 和 scenario 字段

-- 添加 model 字段（存储该 session 使用的模型）
ALTER TABLE sessions ADD COLUMN model TEXT DEFAULT '';

-- 添加 scenario 字段（存储该 session 的场景类型: chat/cron/channel）
ALTER TABLE sessions ADD COLUMN scenario TEXT DEFAULT 'chat';

-- 创建索引以便按场景查询
CREATE INDEX IF NOT EXISTS idx_sessions_scenario ON sessions(scenario);
