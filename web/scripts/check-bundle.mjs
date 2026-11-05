// Checks the built frontend for external URLs (air-gap requirement).
//
// Only strings that are never fetched are allowed: XML namespaces and the
// links in Svelte's error messages. Anything else fails the build, so that
// e.g. a CDN import or a telemetry URL is caught immediately.
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'

const dist = new URL('../dist/', import.meta.url).pathname
const allowed = [
  /^http:\/\/www\.w3\.org\/(1999\/xhtml|2000\/svg|1999\/xlink|XML\/1998\/namespace|1998\/Math\/MathML)$/,
  /^https:\/\/svelte\.dev\/e\/[a-z_]+$/,
]
const urlPattern = /\bhttps?:\/\/[^\s"'`<>)\\]+/g
const textFile = /\.(html|js|css|svg|json|txt|webmanifest)$/

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) yield* walk(p)
    else if (textFile.test(name)) yield p
  }
}

const problems = []
for (const file of walk(dist)) {
  const content = readFileSync(file, 'utf8')
  for (const [url] of content.matchAll(urlPattern)) {
    const clean = url.replace(/[.,;]+$/, '')
    if (!allowed.some((re) => re.test(clean))) problems.push(`${relative(dist, file)}: ${clean}`)
  }
}

if (problems.length > 0) {
  console.error('External URLs found in bundle:\n  ' + [...new Set(problems)].join('\n  '))
  process.exit(1)
}
console.log('Bundle check: no external URLs.')
