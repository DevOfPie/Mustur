#!/usr/bin/env node
// Stub MCP server for the Mustur milestone 1 disproof.
// One tool, `mustur_route`. Every protocol event is appended as a JSON line to
// $MUSTUR_LOG, tagged with $MUSTUR_RUN_ID, so the transcript's claims can be
// checked against what the server actually saw. No dependencies: newline-
// delimited JSON-RPC 2.0 over stdio, per the MCP stdio transport.

'use strict';

const fs = require('fs');

const LOG = process.env.MUSTUR_LOG || '/tmp/mustur-stub.log';
const RUN = process.env.MUSTUR_RUN_ID || 'unset';

function log(event, extra) {
  fs.appendFileSync(
    LOG,
    JSON.stringify({ ts: new Date().toISOString(), run: RUN, event, ...extra }) + '\n'
  );
}

const TOOL = {
  name: 'mustur_route',
  description:
    'Return the routing record for this repository: what it is registered as, ' +
    'which machine holds it, and where its records live. Call once at session ' +
    'start, before any other work.',
  inputSchema: {
    type: 'object',
    properties: {
      repository: { type: 'string', description: 'Repository name as understood from the checkout' },
      task: { type: 'string', description: 'One line on what this session was asked to do' },
    },
    required: ['repository', 'task'],
  },
};

function respond(id, result) {
  process.stdout.write(JSON.stringify({ jsonrpc: '2.0', id, result }) + '\n');
}

let buf = '';
process.stdin.on('data', (chunk) => {
  buf += chunk;
  let nl;
  while ((nl = buf.indexOf('\n')) !== -1) {
    const line = buf.slice(0, nl);
    buf = buf.slice(nl + 1);
    if (!line.trim()) continue;
    let msg;
    try {
      msg = JSON.parse(line);
    } catch {
      log('parse-error', { line: line.slice(0, 200) });
      continue;
    }
    handle(msg);
  }
});

function handle(msg) {
  const { id, method, params } = msg;
  switch (method) {
    case 'initialize':
      log('initialize', { protocolVersion: params && params.protocolVersion });
      respond(id, {
        protocolVersion: (params && params.protocolVersion) || '2025-06-18',
        capabilities: { tools: {} },
        serverInfo: { name: 'mustur', version: '0.0.1-stub' },
      });
      break;
    case 'notifications/initialized':
      log('initialized');
      break;
    case 'tools/list':
      log('tools-list');
      respond(id, { tools: [TOOL] });
      break;
    case 'tools/call': {
      const args = (params && params.arguments) || {};
      log('tools-call', { tool: params && params.name, args });
      respond(id, {
        content: [
          {
            type: 'text',
            text:
              'Routing record: this repository is registered with Mustur. ' +
              'Records for it live in Mustur and are addressable by identifier. ' +
              'No further routing action is needed this session; proceed with the task.',
          },
        ],
      });
      break;
    }
    case 'ping':
      log('ping');
      respond(id, {});
      break;
    default:
      log('unhandled', { method });
      if (id !== undefined) {
        process.stdout.write(
          JSON.stringify({ jsonrpc: '2.0', id, error: { code: -32601, message: 'not implemented' } }) + '\n'
        );
      }
  }
}

log('server-start');
process.stdin.on('end', () => log('server-end'));
