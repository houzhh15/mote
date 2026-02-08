/**
 * useInputHistory - 输入历史记录 Hook
 *
 * 管理用户输入历史，支持上下键导航，使用 localStorage 持久化存储。
 */
import { useState, useCallback, useRef } from 'react';

const STORAGE_KEY = 'mote_input_history';
const DEFAULT_MAX_SIZE = 50;

export interface UseInputHistoryReturn {
  /** 添加新的历史记录 */
  addHistory: (input: string) => void;
  /** 导航到上一条历史记录 */
  navigatePrev: () => string | null;
  /** 导航到下一条历史记录 */
  navigateNext: () => string | null;
  /** 重置导航位置 */
  resetNavigation: () => void;
  /** 获取全部历史记录 */
  history: string[];
  /** 当前导航索引 (-1 表示未导航) */
  currentIndex: number;
  /** 清空所有历史记录 */
  clearHistory: () => void;
}

/**
 * 从 localStorage 加载历史记录
 */
const loadHistory = (): string[] => {
  try {
    const data = localStorage.getItem(STORAGE_KEY);
    if (data) {
      const parsed = JSON.parse(data);
      // 支持两种格式：简单数组或带版本的对象
      if (Array.isArray(parsed)) {
        return parsed;
      }
      if (parsed.history && Array.isArray(parsed.history)) {
        return parsed.history;
      }
    }
    return [];
  } catch (e) {
    console.warn('Failed to load input history:', e);
    return [];
  }
};

/**
 * 保存历史记录到 localStorage
 */
const saveHistory = (history: string[]): void => {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(history));
  } catch (e) {
    console.warn('Failed to save input history:', e);
  }
};

/**
 * 输入历史管理 Hook
 *
 * @param maxSize - 最大存储条数，默认 50
 * @returns 历史记录操作方法和状态
 *
 * @example
 * ```tsx
 * const { addHistory, navigatePrev, navigateNext, resetNavigation } = useInputHistory();
 *
 * // 发送消息后添加历史
 * const handleSend = (message: string) => {
 *   sendMessage(message);
 *   addHistory(message);
 * };
 *
 * // 监听键盘事件
 * const handleKeyDown = (e: KeyboardEvent) => {
 *   if (e.key === 'ArrowUp') {
 *     const prev = navigatePrev();
 *     if (prev !== null) setInput(prev);
 *   } else if (e.key === 'ArrowDown') {
 *     const next = navigateNext();
 *     setInput(next ?? '');
 *   }
 * };
 * ```
 */
export function useInputHistory(maxSize: number = DEFAULT_MAX_SIZE): UseInputHistoryReturn {
  const [history, setHistory] = useState<string[]>(() => loadHistory());
  const [currentIndex, setCurrentIndex] = useState<number>(-1);

  /**
   * 添加新的历史记录
   * - 去重：如果最新一条与新输入相同，则不重复添加
   * - 限制数量：超过 maxSize 时移除最旧的记录
   */
  const addHistory = useCallback(
    (input: string) => {
      const trimmed = input.trim();
      if (!trimmed) return;

      setHistory((prev) => {
        // 去重：如果最新一条相同则不添加
        if (prev.length > 0 && prev[0] === trimmed) {
          return prev;
        }

        // 移除可能存在的相同历史（去重）
        const filtered = prev.filter((item) => item !== trimmed);

        // 添加到开头
        const newHistory = [trimmed, ...filtered].slice(0, maxSize);

        // 持久化
        saveHistory(newHistory);

        return newHistory;
      });

      // 重置导航
      setCurrentIndex(-1);
    },
    [maxSize]
  );

  // 使用 ref 来跟踪 currentIndex，避免闭包问题
  const currentIndexRef = useRef(currentIndex);
  currentIndexRef.current = currentIndex;

  const historyRef = useRef(history);
  historyRef.current = history;

  /**
   * 导航到上一条历史记录
   * 返回上一条历史内容，如果已到顶部则返回 null
   */
  const navigatePrev = useCallback((): string | null => {
    const historyArr = historyRef.current;
    const idx = currentIndexRef.current;
    
    if (historyArr.length === 0) return null;

    let newIndex: number;

    if (idx === -1) {
      // 首次按上键，跳转到最新一条
      newIndex = 0;
    } else if (idx < historyArr.length - 1) {
      // 继续向上
      newIndex = idx + 1;
    } else {
      // 已经到顶部，返回当前内容
      return historyArr[idx];
    }

    // 同时更新 state 和 ref（ref 需要立即更新以便连续调用生效）
    currentIndexRef.current = newIndex;
    setCurrentIndex(newIndex);
    return historyArr[newIndex];
  }, []);

  /**
   * 导航到下一条历史记录
   * 返回下一条历史内容，如果到底部则返回 null（恢复空输入）
   */
  const navigateNext = useCallback((): string | null => {
    const historyArr = historyRef.current;
    const idx = currentIndexRef.current;
    
    if (idx === -1) {
      // 未在导航中
      return null;
    }

    if (idx > 0) {
      // 向下导航
      const newIndex = idx - 1;
      // 同时更新 state 和 ref（ref 需要立即更新以便连续调用生效）
      currentIndexRef.current = newIndex;
      setCurrentIndex(newIndex);
      return historyArr[newIndex];
    } else {
      // 回到底部，恢复空输入
      currentIndexRef.current = -1;
      setCurrentIndex(-1);
      return null;
    }
  }, []);

  /**
   * 重置导航位置
   * 在用户开始新的输入时调用
   */
  const resetNavigation = useCallback(() => {
    setCurrentIndex(-1);
  }, []);

  /**
   * 清空所有历史记录
   */
  const clearHistory = useCallback(() => {
    setHistory([]);
    setCurrentIndex(-1);
    try {
      localStorage.removeItem(STORAGE_KEY);
    } catch (e) {
      console.warn('Failed to clear input history:', e);
    }
  }, []);

  return {
    addHistory,
    navigatePrev,
    navigateNext,
    resetNavigation,
    history,
    currentIndex,
    clearHistory,
  };
}

export default useInputHistory;
