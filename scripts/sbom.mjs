#!/usr/bin/env node
// Writes the software bill of materials of dendronotes as CycloneDX 1.6 JSON:
// the Go modules compiled into the binary (vendor/modules.txt), the Go
// toolchain, and the npm packages bundled into the web interface (runtime
// dependencies in web/package-lock.json; build tools are not shipped).
//
//   node scripts/sbom.mjs [version] > security/sbom.cdx.json
//
// The output is reproducible: no wall-clock time (SOURCE_DATE_EPOCH or the
// last commit), a serial number derived from the content. Licenses of Go
// modules are recognized from their license files; npm packages declare theirs.

import { createHash } from 'node:crypto'
import { execSync } from 'node:child_process'
import { existsSync, readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'

const root = new URL('..', import.meta.url).pathname
const read = (p) => readFileSync(join(root, p), 'utf8')

const version = (process.argv[2] ?? describe()).replace(/^v/, '')
function describe() {
  try {
    return execSync('git describe --tags --always', { cwd: root }).toString().trim()
  } catch {
    return 'dev'
  }
}

function epoch() {
  if (process.env.SOURCE_DATE_EPOCH) return Number(process.env.SOURCE_DATE_EPOCH)
  try {
    return Number(execSync('git log -1 --format=%ct', { cwd: root }).toString().trim())
  } catch {
    return 0
  }
}

// --- Licenses of Go modules --------------------------------------------------

function classify(text) {
  const t = text.replace(/\s+/g, ' ')
  if (/Apache License,? Version 2\.0/i.test(t)) return 'Apache-2.0'
  if (/Mozilla Public License,? (version )?2\.0/i.test(t)) return 'MPL-2.0'
  if (/Permission is hereby granted, free of charge/i.test(t)) return 'MIT'
  if (/Permission to use, copy, modify, and\/?o?r? ?distribute this software for any purpose/i.test(t)) return 'ISC'
  if (/Redistribution and use in source and binary forms/i.test(t)) {
    return /Neither the name|names of its contributors may be used/i.test(t) ? 'BSD-3-Clause' : 'BSD-2-Clause'
  }
  return null
}

function goLicense(module) {
  const dir = join(root, 'vendor', module)
  if (!existsSync(dir)) return null
  const files = readdirSync(dir).filter((f) => /^(LICEN[CS]E|COPYING)(\.(md|txt))?$/i.test(f))
  for (const f of files) {
    const id = classify(readFileSync(join(dir, f), 'utf8'))
    if (id) return id
  }
  return null
}

// --- Components -----------------------------------------------------------------

const components = []
const unknown = []

const goMod = read('go.mod')
const goVersion = (/^toolchain go(\S+)/m.exec(goMod) ?? /^go (\S+)/m.exec(goMod))[1]
components.push({
  type: 'library',
  'bom-ref': `pkg:golang/stdlib@v${goVersion}`,
  name: 'stdlib',
  group: 'go',
  version: `v${goVersion}`,
  description: 'The Go standard library and runtime compiled into the binary',
  purl: `pkg:golang/stdlib@v${goVersion}`,
  licenses: [{ license: { id: 'BSD-3-Clause' } }],
})

for (const line of read('vendor/modules.txt').split('\n')) {
  const m = /^# (\S+) (v\S+)(?: => (\S+) (v\S+))?$/.exec(line)
  if (!m) continue
  const [, path, ver, replPath, replVer] = m
  const name = replPath ?? path
  const v = replVer ?? ver
  const license = goLicense(path)
  if (!license) unknown.push(`${path}@${ver}`)
  components.push({
    type: 'library',
    'bom-ref': `pkg:golang/${name}@${v}`,
    name,
    version: v,
    purl: `pkg:golang/${name}@${v}`,
    ...(license ? { licenses: [{ license: { id: license } }] } : {}),
  })
}

const lock = JSON.parse(read('web/package-lock.json'))
for (const [key, p] of Object.entries(lock.packages ?? {})) {
  if (!key.startsWith('node_modules/') || p.dev || p.devOptional) continue
  const name = key.slice(key.lastIndexOf('node_modules/') + 'node_modules/'.length)
  const purl = `pkg:npm/${name.replace(/^@/, '%40')}@${p.version}`
  if (!p.license) unknown.push(`${name}@${p.version}`)
  components.push({
    type: 'library',
    'bom-ref': purl,
    name,
    version: p.version,
    purl,
    ...(p.license ? { licenses: [{ expression: p.license }] } : {}),
    ...(p.integrity ? { hashes: [integrityHash(p.integrity)] } : {}),
  })
}

function integrityHash(integrity) {
  const [alg, b64] = integrity.split('-')
  const map = { sha512: 'SHA-512', sha384: 'SHA-384', sha256: 'SHA-256', sha1: 'SHA-1' }
  return { alg: map[alg] ?? alg, content: Buffer.from(b64, 'base64').toString('hex') }
}

components.sort((a, b) => a['bom-ref'].localeCompare(b['bom-ref']))
const seen = new Set()
const unique = components.filter((c) => !seen.has(c['bom-ref']) && seen.add(c['bom-ref']))

const app = {
  type: 'application',
  'bom-ref': `pkg:github/erktm/dendronotes@v${version}`,
  name: 'dendronotes',
  version,
  description: 'Self-hosted Markdown notes',
  licenses: [{ license: { id: 'Apache-2.0' } }],
  purl: `pkg:github/erktm/dendronotes@v${version}`,
  externalReferences: [{ type: 'vcs', url: 'https://github.com/erktm/dendronotes' }],
}

const bom = {
  bomFormat: 'CycloneDX',
  specVersion: '1.6',
  serialNumber: '',
  version: 1,
  metadata: {
    timestamp: new Date(epoch() * 1000).toISOString().replace(/\.\d+Z$/, 'Z'),
    tools: { components: [{ type: 'application', name: 'dendronotes scripts/sbom.mjs' }] },
    component: app,
  },
  components: unique,
  dependencies: [{ ref: app['bom-ref'], dependsOn: unique.map((c) => c['bom-ref']) }],
}

// A serial number that only changes with the content.
const digest = createHash('sha256').update(JSON.stringify(bom)).digest('hex')
bom.serialNumber = `urn:uuid:${digest.slice(0, 8)}-${digest.slice(8, 12)}-5${digest.slice(13, 16)}-${((parseInt(digest[16], 16) & 3) | 8).toString(16)}${digest.slice(17, 20)}-${digest.slice(20, 32)}`

process.stdout.write(JSON.stringify(bom, null, 2) + '\n')
if (unknown.length) process.stderr.write(`sbom: no license recognized for ${unknown.length} components:\n  ${unknown.join('\n  ')}\n`)
