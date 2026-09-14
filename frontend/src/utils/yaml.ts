import { dump as yamlDump, load as yamlLoad } from 'js-yaml';

/** 将资源对象序列化为可读 YAML；兼容 unstructured.Object 包装并去掉 managedFields。 */
export function resourceToYaml(resource: unknown): string {
  if (resource == null) return '';
  if (typeof resource === 'string') return resource;
  if (typeof resource !== 'object') return String(resource);

  const raw = resource as Record<string, unknown>;
  const wrapped = raw.Object;
  const obj: Record<string, unknown> =
    wrapped && typeof wrapped === 'object' && !Array.isArray(wrapped) && !raw.apiVersion && !raw.kind
      ? (wrapped as Record<string, unknown>)
      : raw;

  let clone: Record<string, unknown>;
  try {
    clone = JSON.parse(JSON.stringify(obj)) as Record<string, unknown>;
  } catch {
    clone = { ...obj };
  }

  const meta = clone.metadata;
  if (meta && typeof meta === 'object' && !Array.isArray(meta)) {
    delete (meta as Record<string, unknown>).managedFields;
  }

  try {
    return yamlDump(clone, { lineWidth: 120, noRefs: true });
  } catch {
    return JSON.stringify(clone, null, 2);
  }
}

export function tryParseYaml(text: string): unknown | null {
  try {
    return yamlLoad(text);
  } catch {
    return null;
  }
}
