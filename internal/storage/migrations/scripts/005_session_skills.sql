-- 005_session_skills.sql: 添加 session selected_skills 列

-- 为 sessions 表添加 selected_skills 列（JSON数组格式，存储选中的skill ID列表）
-- 空字符串表示"全部"（默认行为），非空JSON数组表示手动选择
ALTER TABLE sessions ADD COLUMN selected_skills TEXT DEFAULT '';
