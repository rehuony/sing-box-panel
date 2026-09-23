import type { RJSFSchema } from '@rjsf/utils';

import { describe, expect, it } from 'vitest';

import { configurationFieldCopy } from '@/i18n/locales/zh-CN/configuration-fields';
import { schemaProperties, selfContainedSchema } from '@/pages/configuration-page/schema-ui';
import { configurationFieldHelp, readConfigurationFieldHelp } from '@/pages/configuration-page/configuration-field-help';

const schemas = import.meta.glob<RJSFSchema>('../../../schemas/generated/schema-*.json', { eager: true, import: 'default' });

describe('configuration field help', () => {
  it('covers every declared property in every committed exact-version schema', () => {
    const missing = new Set<string>();
    const keys = new Set<string>();
    function visit(value: unknown) {
      if (Array.isArray(value)) {
        value.forEach(visit);
      } else if (value !== null && typeof value === 'object') {
        const record = value as RJSFSchema;
        for (const key of Object.keys(record.properties ?? {})) {
          keys.add(key);
          const copy = configurationFieldHelp(key, []);
          if (!copy || !/[\u4E00-\u9FFF]/u.test(copy[1]) || copy[1].length < 8) missing.add(key);
        }
        Object.values(record).forEach(visit);
      }
    }
    expect(Object.keys(schemas)).toHaveLength(5);
    Object.values(schemas).forEach(visit);
    expect([...missing]).toEqual([]);
    expect(new Set(Object.keys(configurationFieldCopy))).toEqual(keys);
  });

  it('distinguishes ambiguous keys by section, definition and protocol', () => {
    expect(configurationFieldHelp('final', ['dns'])?.[0]).toBe('默认 DNS 服务器');
    expect(configurationFieldHelp('final', ['route'])?.[0]).toBe('默认出站');
    expect(configurationFieldHelp('server', ['DomainResolver'])?.[0]).toBe('DNS 服务器标签');
    expect(configurationFieldHelp('server', ['Outbound', 'trojan'])?.[0]).toBe('服务器地址');
    expect(configurationFieldHelp('method', ['Outbound', 'shadowsocks'])?.[0]).toBe('加密方法');
    expect(configurationFieldHelp('method', ['DNSRuleAction', 'reject'])?.[1]).toContain('REFUSED');
    expect(configurationFieldHelp('method', ['RuleAction', 'reject'])?.[1]).not.toContain('REFUSED');
    expect(configurationFieldHelp('timeout', ['DNS', 'optimistic'])?.[0]).toBe('过期缓存可用时长');
    expect(configurationFieldHelp('timeout', ['RuleAction', 'sniff'])?.[0]).toBe('协议探测超时');
    expect(configurationFieldHelp('future_key', [])).toBeUndefined();
    expect(configurationFieldHelp('constructor', [])).toBeUndefined();
  });

  it.each(Object.entries(schemas))('annotates reference and union fields without mutating %s', (_, root) => {
    const original = JSON.stringify(root);
    const properties = schemaProperties(root, root);
    for (const [section, title] of [['dns', '默认 DNS 服务器'], ['route', '默认出站']]) {
      const presentation = selfContainedSchema(properties[section], root, {}, [section]);
      expect(readConfigurationFieldHelp(schemaProperties(presentation, presentation).final)?.label).toBe(title);
    }
    const presentation = selfContainedSchema(root, root);
    const definitions = presentation.$defs as Record<string, RJSFSchema>;
    const action = definitions.DNSRuleAction;
    const server = schemaProperties(action, presentation, { action: 'route', server: 'dns-local' }).server;
    expect(readConfigurationFieldHelp(server)?.label).toBe('DNS 服务器标签');
    expect(JSON.stringify(root)).toBe(original);
  });
});
