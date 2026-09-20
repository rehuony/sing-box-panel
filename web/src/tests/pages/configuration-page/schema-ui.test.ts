import type { RJSFSchema } from '@rjsf/utils';

import { describe, expect, it } from 'vitest';

import {
  collectionItemSchema,
  mergeSchemaKnownData,
  projectSchemaKnownData,
  schemaDiscriminatorValues,
  schemaProperties,
  selfContainedSchema,
} from '@/pages/configuration-page/schema-ui';

const schema: RJSFSchema = {
  type: 'object',
  properties: {
    dns_server: { $ref: '#/$defs/DNSServer' },
    labels: { $ref: '#/$defs/OpenMap' },
    opaque: { type: 'object', additionalProperties: true },
    rule: { $ref: '#/$defs/Rule' },
    inbounds: {
      type: 'array',
      items: { $ref: '#/$defs/Inbound' },
    },
  },
  $defs: {
    DNSServer: {
      oneOf: [
        {
          type: 'object',
          additionalProperties: false,
          properties: {
            address: { type: 'string' },
            type: { type: 'string', enum: ['', 'legacy'] },
          },
        },
        {
          type: 'object',
          additionalProperties: false,
          properties: { interface: { type: 'string' }, type: { const: 'dhcp' } },
          required: ['type'],
        },
      ],
    },
    Inbound: {
      oneOf: [
        {
          type: 'object',
          additionalProperties: false,
          properties: {
            listen: { type: 'string' },
            tag: { type: 'string' },
            tls: { $ref: '#/$defs/TLS' },
            type: { const: 'mixed' },
          },
          required: ['type'],
        },
        {
          type: 'object',
          additionalProperties: false,
          properties: {
            tag: { type: 'string' },
            type: { const: 'http' },
            users: { type: 'array', items: { $ref: '#/$defs/User' } },
          },
          required: ['type'],
        },
      ],
    },
    TLS: {
      type: 'object',
      additionalProperties: false,
      properties: { enabled: { type: 'boolean' }, server_name: { type: 'string' } },
    },
    User: {
      type: 'object',
      additionalProperties: false,
      properties: { name: { type: 'string' }, password: { type: 'string' } },
    },
    OpenMap: {
      type: 'object',
      additionalProperties: {
        type: 'object',
        additionalProperties: false,
        properties: { value: { type: 'string' } },
      },
    },
    Rule: {
      oneOf: [
        {
          allOf: [
            { type: 'object', properties: { type: { enum: ['', 'default'] } } },
            { $ref: '#/$defs/RuleAction' },
          ],
        },
        {
          type: 'object',
          properties: { mode: { type: 'string' }, type: { const: 'logical' } },
          required: ['type', 'mode'],
        },
      ],
    },
    RuleAction: {
      oneOf: [
        { type: 'object', properties: { action: { const: 'route' }, server: { type: 'string' } } },
        {
          type: 'object',
          properties: { action: { const: 'reject' }, method: { type: 'string' } },
          required: ['action'],
        },
      ],
    },
  },
};

describe('schemaUi', () => {
  it('resolves local item refs and enumerates discriminator constants from unions', () => {
    const inbounds = schemaProperties(schema).inbounds;
    const item = collectionItemSchema(inbounds, schema);

    expect(item).not.toBeNull();
    expect(schemaDiscriminatorValues(item!, schema, 'type')).toEqual(['mixed', 'http']);
    expect(Object.keys(schemaProperties(item!, schema, { type: 'mixed' }))).toEqual(
      expect.arrayContaining(['listen', 'tag', 'tls', 'type']),
    );
    expect(schemaProperties(item!, schema, { type: 'mixed' })).not.toHaveProperty('users');
  });

  it('makes a selected union branch self-contained for RJSF local refs', () => {
    const inbounds = schemaProperties(schema).inbounds;
    const item = collectionItemSchema(inbounds, schema)!;
    const section = selfContainedSchema(item, schema, { type: 'mixed' });

    expect(section.oneOf).toBeUndefined();
    expect(section.$defs).toMatchObject(schema.$defs!);
    expect(section.$defs).toHaveProperty('Inbound.discriminator.propertyName', 'type');
    expect(schema.$defs?.Inbound).not.toHaveProperty('discriminator');
    expect(section.properties).toHaveProperty('tls.$ref', '#/$defs/TLS');
  });

  it('projects nested known data through the selected branch without panel metadata', () => {
    expect(projectSchemaKnownData(schema, schema, {
      future_root: { retained: true },
      inbounds: [
        {
          future_protocol: { retained: true },
          type: 'http',
        },
        {
          future_protocol: { retained: true },
          listen: '127.0.0.1',
          tag: 'active',
          tls: { enabled: true, future_tls: true, server_name: 'edge.example' },
          type: 'mixed',
          users: [{ name: 'wrong-branch' }],
        },
      ],
    })).toEqual({
      inbounds: [
        { type: 'http' },
        {
          listen: '127.0.0.1',
          tag: 'active',
          tls: { enabled: true, server_name: 'edge.example' },
          type: 'mixed',
        },
      ],
    });
  });

  it('selects an optional-discriminator default and retains schema-declared open maps', () => {
    expect(projectSchemaKnownData(schema, schema, {
      dns_server: { address: 'local', future_dns: true },
      labels: {
        first: { future_label: true, value: 'one' },
        second: { value: 'two' },
      },
      opaque: { nested: { remains: 'lossless' } },
    })).toEqual({
      dns_server: { address: 'local' },
      labels: { first: { value: 'one' }, second: { value: 'two' } },
      opaque: { nested: { remains: 'lossless' } },
    });
  });

  it('selects a discriminator nested through allOf and a referenced union', () => {
    expect(projectSchemaKnownData(schema, schema, {
      rule: { action: 'reject', future_rule: true, method: 'drop' },
    })).toEqual({ rule: { action: 'reject', method: 'drop' } });
  });

  it('preserves unknown fields on retained array members while deleting removed members', () => {
    const original = {
      inbounds: [{
        type: 'http',
        users: [
          { future_user: 'remove-with-alice', name: 'alice', password: 'a' },
          { future_user: 'keep-with-bob', name: 'bob', password: 'b' },
        ],
      }],
    };
    const before = projectSchemaKnownData(schema, schema, original);
    const after = {
      inbounds: [{
        type: 'http',
        users: [
          { name: 'bob', password: 'b' },
          { name: 'charlie', password: 'c' },
        ],
      }],
    };

    expect(mergeSchemaKnownData(schema, schema, original, before, after)).toEqual({
      inbounds: [{
        type: 'http',
        users: [
          { future_user: 'keep-with-bob', name: 'bob', password: 'b' },
          { name: 'charlie', password: 'c' },
        ],
      }],
    });
  });
});
