import React, { useCallback, useEffect, useRef, useState } from 'react';
import Editor, { DiffEditor } from '@monaco-editor/react';
import type { editor } from 'monaco-editor';
import '@/app/monaco';

const DEFAULT_OPTIONS: editor.IStandaloneEditorConstructionOptions = {
  minimap: { enabled: false },
  fontSize: 13,
  lineNumbers: 'on',
  automaticLayout: true,
  scrollBeyondLastLine: false,
  renderWhitespace: 'boundary',
  tabSize: 2,
  insertSpaces: true,
  wordWrap: 'on',
};

function resolveDefaultHeight(): number {
  if (typeof window === 'undefined') return 520;
  return Math.max(480, window.innerHeight - 280);
}

export interface YamlEditorProps {
  value: string;
  onChange?: (value: string) => void;
  readOnly?: boolean;
  /** 容器高度；百分比/calc 时内部会转成像素，避免 Monaco 高度为 0 */
  height?: number | string;
  theme?: string;
  editorKey?: string;
}

export const YamlEditor: React.FC<YamlEditorProps> = ({
  value,
  onChange,
  readOnly,
  height = 'calc(100vh - 280px)',
  theme = 'vs',
  editorKey,
}) => {
  const hostRef = useRef<HTMLDivElement>(null);
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null);
  const [pxHeight, setPxHeight] = useState<number>(
    typeof height === 'number' ? height : resolveDefaultHeight(),
  );

  const syncHeight = useCallback(() => {
    const el = hostRef.current;
    if (!el) return;
    const h = el.clientHeight;
    if (h > 40) {
      setPxHeight(h);
    } else if (typeof height === 'number') {
      setPxHeight(height);
    } else {
      setPxHeight(resolveDefaultHeight());
    }
    editorRef.current?.layout();
  }, [height]);

  useEffect(() => {
    syncHeight();
    const el = hostRef.current;
    if (!el || typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', syncHeight);
      return () => window.removeEventListener('resize', syncHeight);
    }
    const ro = new ResizeObserver(() => syncHeight());
    ro.observe(el);
    window.addEventListener('resize', syncHeight);
    return () => {
      ro.disconnect();
      window.removeEventListener('resize', syncHeight);
    };
  }, [syncHeight]);

  return (
    <div
      ref={hostRef}
      className="yaml-editor-host"
      style={{
        height: typeof height === 'number' ? height : height,
        minHeight: 420,
        width: '100%',
      }}
    >
      <Editor
        key={editorKey}
        height={pxHeight}
        defaultLanguage="yaml"
        language="yaml"
        theme={theme}
        value={value}
        onChange={(v) => onChange?.(v ?? '')}
        onMount={(ed) => {
          editorRef.current = ed;
          requestAnimationFrame(() => {
            ed.layout();
            // 数据后到时，强制把受控 value 写进编辑器，避免首次高度为 0 时内容丢失
            if (value && ed.getValue() !== value) {
              ed.setValue(value);
            }
          });
        }}
        options={{ ...DEFAULT_OPTIONS, readOnly: !!readOnly }}
        loading={<div style={{ padding: 24, color: 'rgba(0,0,0,0.45)' }}>加载编辑器...</div>}
      />
    </div>
  );
};

export interface YamlDiffEditorProps {
  original: string;
  modified: string;
  height?: number | string;
  theme?: string;
}

export const YamlDiffEditor: React.FC<YamlDiffEditorProps> = ({
  original,
  modified,
  height = 'calc(100vh - 220px)',
  theme = 'vs',
}) => {
  const hostRef = useRef<HTMLDivElement>(null);
  const editorRef = useRef<editor.IStandaloneDiffEditor | null>(null);
  const [pxHeight, setPxHeight] = useState<number>(
    typeof height === 'number' ? height : resolveDefaultHeight(),
  );

  const syncHeight = useCallback(() => {
    const el = hostRef.current;
    if (!el) return;
    const h = el.clientHeight;
    if (h > 40) setPxHeight(h);
    else setPxHeight(typeof height === 'number' ? height : resolveDefaultHeight());
    editorRef.current?.layout();
  }, [height]);

  useEffect(() => {
    syncHeight();
    const el = hostRef.current;
    if (!el || typeof ResizeObserver === 'undefined') return undefined;
    const ro = new ResizeObserver(() => syncHeight());
    ro.observe(el);
    window.addEventListener('resize', syncHeight);
    return () => {
      ro.disconnect();
      window.removeEventListener('resize', syncHeight);
    };
  }, [syncHeight]);

  return (
    <div
      ref={hostRef}
      className="yaml-editor-host"
      style={{
        height: typeof height === 'number' ? height : height,
        minHeight: 420,
        width: '100%',
      }}
    >
      <DiffEditor
        height={pxHeight}
        original={original}
        modified={modified}
        originalLanguage="yaml"
        modifiedLanguage="yaml"
        theme={theme}
        onMount={(ed) => {
          editorRef.current = ed;
          requestAnimationFrame(() => ed.layout());
        }}
        options={{
          minimap: { enabled: false },
          fontSize: 13,
          readOnly: true,
          automaticLayout: true,
          renderSideBySide: true,
          wordWrap: 'on',
        }}
      />
    </div>
  );
};
