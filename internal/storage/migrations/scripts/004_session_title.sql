-- 004_session_title.sql: 添加 session title 列

-- 为 sessions 表添加 title 列
ALTER TABLE sessions ADD COLUMN title TEXT DEFAULT '';
