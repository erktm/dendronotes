#!/usr/bin/env node
// Compares what the vulnerability scanners find with the statements in
// security/vex.openvex.json and fails when a finding has no statement.
//
//   go run golang.org/x/vuln/cmd/govulncheck@latest -format json ./... > govulncheck.json
//   (cd web && npm audit --json) > npm-audit.json
//   node scripts/vex-check.mjs govulncheck.json npm-audit.json
//
// A finding is covered when a statement names its id (GO-…, GHSA-…, CVE-…)
// or one of its aliases. Add a statement with status and justification for
// every new finding, or upgrade the dependency.

import { readFileSync } from 'node:fs'

const [goFile, npmFile, vexFile = new URL('../security/vex.openvex.json', import.meta.url).pathname] = process.argv.slice(2)
if (!goFile || !npmFile) {
  console.error('usage: vex-check.mjs <govulncheck.json> <npm-audit.json> [vex.json]')
  process.exit(2)
}

const vex = JSON.parse(readFileSync(vexFile, 'utf8'))
const covered = new Set(vex.statements.flatMap((s) => [s.vulnerability.name, ...(s.vulnerability.aliases ?? [])]))

/** govulncheck -format json is a stream of JSON objects. */
function stream(text) {
  const out = []
  let depth = 0
  let start = -1
  let inString = false
  for (let i = 0; i < text.length; i++) {
    const c = text[i]
    if (inString) {
      if (c === '\\') i++
      else if (c === '"') inString = false
      continue
    }
    if (c === '"') inString = true
    else if (c === '{') {
      if (depth++ === 0) start = i
    } else if (c === '}' && --depth === 0) out.push(JSON.parse(text.slice(start, i + 1)))
  }
  return out
}

const findings = new Map()
const goObjects = stream(readFileSync(goFile, 'utf8'))
const osv = new Map(goObjects.filter((o) => o.osv).map((o) => [o.osv.id, o.osv]))
for (const o of goObjects) {
  if (!o.finding) continue
  const id = o.finding.osv
  findings.set(id, [id, ...(osv.get(id)?.aliases ?? [])])
}

const audit = JSON.parse(readFileSync(npmFile, 'utf8'))
for (const v of Object.values(audit.vulnerabilities ?? {})) {
  for (const via of v.via) {
    if (typeof via !== 'object' || !via.url) continue
    const id = via.url.split('/').pop()
    findings.set(id, [id])
  }
}

const missing = [...findings.values()].filter((ids) => !ids.some((id) => covered.has(id)))
for (const [id, ...aliases] of findings.values()) {
  const ok = !missing.some((m) => m[0] === id)
  console.log(`${ok ? 'covered ' : 'MISSING '} ${id}${aliases.length ? ` (${aliases.join(', ')})` : ''}`)
}
if (missing.length) {
  console.error(`\n${missing.length} finding(s) without a statement in ${vexFile}.`)
  process.exit(1)
}
console.log(`\nAll ${findings.size} finding(s) have a VEX statement.`)
