/**
 * Monaco Editor 本地化加载配置（全应用共享，各编辑器页面统一 import 本模块）
 *
 * 背景：@monaco-editor/react 默认从 CDN（cdn.jsdelivr.net）加载 monaco，
 * 内网/离线环境（如 VMware 虚拟机）无法访问外网时，编辑器会永远停留在
 * 「加载编辑器中」。此处改为直接使用 npm 依赖本地打包的 monaco-editor，
 * 并为 Vite 配置 web worker，保证任何网络环境下都可用。
 */
import { loader } from '@monaco-editor/react';
import * as monaco from 'monaco-editor';
import editorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker';
import 'monaco-editor/esm/vs/basic-languages/yaml/yaml.contribution';

// Vite 环境下 Monaco 的 web worker 通过 ?worker 导入构造
self.MonacoEnvironment = {
  getWorker(): Worker {
    return new editorWorker();
  },
};

// 让 @monaco-editor/react 使用本地打包的 monaco 实例，而非 CDN
loader.config({ monaco });
void loader.init();

export default monaco;
