import type { RJSFSchema } from '@rjsf/utils';

import { resolve } from 'node:path';
import { readFileSync } from 'node:fs';
import { parse, stringify } from 'lossless-json';
import { beforeAll, describe, expect, it } from 'vitest';

import type { CanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

import { reviewedSchemaManifest } from '@/schemas/generated';
import { resolveReviewedSchema } from '@/schemas/resolve-reviewed-schema';
import { representativeSchemaVersions } from '@/tests/schemas/schema-fixtures';
import { visibleStructuredConfiguration } from '@/pages/configuration-page/structured-validation';
import { encodeCanonicalDraft, parseCanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';
import {
  collectionItemSchema,
  mergeSchemaKnownData,
  projectSchemaKnownData,
  schemaDiscriminatorValues,
  schemaProperties,
  selfContainedSchema,
} from '@/pages/configuration-page/schema-ui';

it('preserves numeric lexemes when records without identities are reordered through a rounded display projection', () => {
  const arraySchema: RJSFSchema = {
    type: 'array', items: { type: 'object', properties: { counter: { type: 'integer' } } },
  };
  const original = parse('[{"counter":900719925474099312345,"future":"first"},{"counter":2,"future":"second"}]');
  const before = JSON.parse(stringify(projectSchemaKnownData(arraySchema, arraySchema, original))!);
  const result = mergeSchemaKnownData(arraySchema, arraySchema, original, before, [before[1], before[0]]);
  expect(stringify(result)).toBe('[{"counter":2,"future":"second"},{"counter":900719925474099312345,"future":"first"}]');
});

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
  it('flattens only unconstrained anyOf wrappers in the presentation schema', () => {
    const wrapped: RJSFSchema = { anyOf: [{ type: 'string' }, { type: 'integer' }] };
    const constrained: RJSFSchema = { ...wrapped, title: 'Keep this choice', minimum: 1 };
    const source: RJSFSchema = {
      type: 'object',
      properties: {
        flat: { anyOf: [wrapped, { type: 'array', items: wrapped }] },
        constrained: { anyOf: [constrained, { type: 'null' }] },
        exclusive: { oneOf: [{ oneOf: [{ type: 'number' }, { type: 'integer' }] }, { type: 'null' }] },
      },
    };
    const result = selfContainedSchema(source, source);
    expect(result.properties?.flat).toMatchObject({ anyOf: [{ type: 'string' }, { type: 'integer' }, { type: 'array' }] });
    expect(result.properties?.constrained).toEqual(source.properties?.constrained);
    expect(result.properties?.exclusive).toEqual(source.properties?.exclusive);
    expect(source.properties?.flat).toEqual({ anyOf: [wrapped, { type: 'array', items: wrapped }] });
  });

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

it('validates only fields declared by the version schema', () => {
  expect(visibleStructuredConfiguration({
    type: 'object', properties: { log: { type: 'object', properties: { level: { type: 'string' } } } },
  }, { future: true, log: { future: 'retained', level: 'info' } })).toEqual({ log: { level: 'info' } });
});

const fixtures = [
  'dns-actions.json', 'dns-legacy.json', 'tls-transport.json', 'endpoints-services.json',
  'null-sections.json', 'log-defaults.json', 'log-null-fields.json',
].map((name) => ({
  name,
  text: readFileSync(resolve(process.cwd(), `../internal/singbox/testdata/configuration-1.13/${name}`), 'utf8'),
}));

beforeAll(async () => {
  await Promise.all(representativeSchemaVersions('reviewed-1.13').map(version => reviewedSchemaManifest[version].load()));
});

describe.each(representativeSchemaVersions('reviewed-1.13'))('reviewed 1.13 projection %s', version => {
  it('validates every section and preserves DNS, TLS, rules and protocols through form projection', async () => {
    const loaded = await reviewedSchemaManifest[version].load();
    const resolution = await resolveReviewedSchema({
      exact_version: version, schema_sha256: loaded.schemaSHA256, schema: loaded.schema,
    }, version);
    const validate = Object.values(loaded.validateFns)[0];
    for (const fixture of fixtures) {
      const draft = parseCanonicalDraft(fixture.text);
      expect(validate(JSON.parse(fixture.text)), fixture.name).toBe(true);
      const projected = projectSchemaKnownData(resolution.schema, resolution.schema, draft);
      const merged = mergeSchemaKnownData(
        resolution.schema, resolution.schema, draft, projected, structuredClone(projected),
      );
      expect(JSON.parse(encodeCanonicalDraft(merged as CanonicalDraft)), fixture.name)
        .toEqual(JSON.parse(fixture.text));
    }
    for (const value of [
      { outbounds: {} },
      { log: [] },
      { log: { disabled: 'false' } },
      { log: { level: 'verbose' } },
      { log: { colour: true } },
      { certificate: { providers: [{ type: 'acme', tag: 'cert' }] } },
      { inbounds: [{ type: 'snell', listen_port: 2080, psk: 'secret' }] },
      { endpoints: [{ type: 'openvpn', tag: 'vpn' }] },
      { dns: { servers: [{ type: 'mdns' }] } },
    ]) expect(validate(value)).toBe(false);
  });
});
