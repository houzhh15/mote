/**
 * useSessionGroups - 会话分组 Hook
 *
 * 根据会话的更新时间将会话列表分组为：Today、Yesterday、Previous 7 Days、Older
 */
import { useMemo } from 'react';
import type { Session } from '../types';

export interface SessionGroup {
  key: 'today' | 'yesterday' | 'week' | 'older';
  label: string;
  sessions: Session[];
}

/**
 * 获取日期的开始时间（0:00:00）
 */
const getStartOfDay = (date: Date): Date => {
  const d = new Date(date);
  d.setHours(0, 0, 0, 0);
  return d;
};

/**
 * 判断两个日期是否是同一天
 */
const isSameDay = (date1: Date, date2: Date): boolean => {
  return (
    date1.getFullYear() === date2.getFullYear() &&
    date1.getMonth() === date2.getMonth() &&
    date1.getDate() === date2.getDate()
  );
};

/**
 * 判断日期是否是昨天
 */
const isYesterday = (date: Date, today: Date): boolean => {
  const yesterday = new Date(today);
  yesterday.setDate(yesterday.getDate() - 1);
  return isSameDay(date, yesterday);
};

/**
 * 判断日期是否在过去7天内（不包括今天和昨天）
 */
const isWithinWeek = (date: Date, today: Date): boolean => {
  const dayStart = getStartOfDay(today);
  const weekAgo = new Date(dayStart);
  weekAgo.setDate(weekAgo.getDate() - 7);

  const twoDaysAgo = new Date(dayStart);
  twoDaysAgo.setDate(twoDaysAgo.getDate() - 2);

  const targetStart = getStartOfDay(date);

  // 在2天前到7天前之间
  return targetStart >= weekAgo && targetStart <= twoDaysAgo;
};

/**
 * 会话分组 Hook
 *
 * @param sessions - 会话列表
 * @returns 分组后的会话数组
 *
 * @example
 * ```tsx
 * const groups = useSessionGroups(sessions);
 *
 * return (
 *   <div>
 *     {groups.map(group => (
 *       <div key={group.key}>
 *         <h4>{group.label}</h4>
 *         {group.sessions.map(session => (
 *           <SessionItem key={session.id} session={session} />
 *         ))}
 *       </div>
 *     ))}
 *   </div>
 * );
 * ```
 */
export function useSessionGroups(sessions: Session[]): SessionGroup[] {
  return useMemo(() => {
    const today = new Date();
    const todaySessions: Session[] = [];
    const yesterdaySessions: Session[] = [];
    const weekSessions: Session[] = [];
    const olderSessions: Session[] = [];

    // 先按更新时间倒序排序
    const sorted = [...sessions].sort((a, b) => {
      const timeA = new Date(a.updated_at).getTime();
      const timeB = new Date(b.updated_at).getTime();
      return timeB - timeA;
    });

    // 分组
    for (const session of sorted) {
      const sessionDate = new Date(session.updated_at);

      if (isSameDay(sessionDate, today)) {
        todaySessions.push(session);
      } else if (isYesterday(sessionDate, today)) {
        yesterdaySessions.push(session);
      } else if (isWithinWeek(sessionDate, today)) {
        weekSessions.push(session);
      } else {
        olderSessions.push(session);
      }
    }

    // 构建结果，过滤空分组
    const groups: SessionGroup[] = [];

    if (todaySessions.length > 0) {
      groups.push({
        key: 'today',
        label: 'Today',
        sessions: todaySessions,
      });
    }

    if (yesterdaySessions.length > 0) {
      groups.push({
        key: 'yesterday',
        label: 'Yesterday',
        sessions: yesterdaySessions,
      });
    }

    if (weekSessions.length > 0) {
      groups.push({
        key: 'week',
        label: 'Previous 7 Days',
        sessions: weekSessions,
      });
    }

    if (olderSessions.length > 0) {
      groups.push({
        key: 'older',
        label: 'Older',
        sessions: olderSessions,
      });
    }

    return groups;
  }, [sessions]);
}

export default useSessionGroups;
