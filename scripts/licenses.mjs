#!/usr/bin/env node
// Writes the license texts of everything compiled or bundled into dendronotes:
// the Go standard library, the vendored Go modules (vendor/modules.txt) and
// the npm packages bundled into the web interface. Their licenses (MIT, BSD,
// Apache-2.0, ISC, the SIL Open Font License and others) ask for these notices to go
// along with binaries, so releases and the image ship this file.
//
//   node scripts/licenses.mjs > THIRD_PARTY_LICENSES.txt

import { existsSync, readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'

const root = new URL('..', import.meta.url).pathname
const read = (p) => readFileSync(join(root, p), 'utf8')
const MIT = `Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.`
const NOTICE_FILES = /^(LICEN[CS]E|COPYING|NOTICE|COPYRIGHT)([.-][A-Za-z0-9]+)*$/i

/** The license and notice texts in a package folder. */
function texts(dir) {
  if (!existsSync(dir)) return []
  return readdirSync(dir)
    .filter((f) => NOTICE_FILES.test(f))
    .sort()
    .map((f) => readFileSync(join(dir, f), 'utf8').replace(/\r\n/g, '\n').trimEnd())
}

const sections = []
const missing = []

function add(title, license, found) {
  sections.push(
    `${'='.repeat(78)}\n${title}${license ? ` (${license})` : ''}\n${'='.repeat(78)}\n\n` +
      (found.length ? found.join('\n\n' + '-'.repeat(40) + '\n\n') : `${license}: see the package for the license text.`),
  )
  if (!found.length) missing.push(title)
}

// Go standard library and runtime: the Go license, the same text the
// golang.org/x modules carry (installations do not always include it).
add('Go standard library and runtime (go.dev)', 'BSD-3-Clause', texts(join(root, 'vendor/golang.org/x/sys')))

// Vendored Go modules.
for (const line of read('vendor/modules.txt').split('\n')) {
  const m = /^# (\S+) (v\S+)/.exec(line)
  if (m) add(`${m[1]} ${m[2]}`, '', texts(join(root, 'vendor', m[1])))
}

// npm packages bundled into the web interface (not the build tools).
const lock = JSON.parse(read('web/package-lock.json'))
for (const [key, p] of Object.entries(lock.packages ?? {}).sort(([a], [b]) => a.localeCompare(b))) {
  if (!key.startsWith('node_modules/') || p.dev || p.devOptional) continue
  const name = key.slice(key.lastIndexOf('node_modules/') + 'node_modules/'.length)
  let found = texts(join(root, 'web', key))
  if (!found.length && p.license === 'MIT') found = [mitText(join(root, 'web', key))]
  add(`${name} ${p.version}`, p.license ?? '', found)
}

/** The MIT license for a package that declares it without shipping its text. */
function mitText(dir) {
  const pkg = JSON.parse(readFileSync(join(dir, 'package.json'), 'utf8'))
  const author = typeof pkg.author === 'string' ? pkg.author : (pkg.author?.name ?? 'the package authors')
  return `The package declares the MIT license without including its text.\n\nCopyright (c) ${author}\n\n${MIT}`
}

process.stdout.write(
  `Third-party software in dendronotes\n\n` +
    `dendronotes contains the following software. Each part is distributed under\n` +
    `its own license, reproduced below with its copyright notices.\n\n` +
    sections.join('\n\n') +
    '\n',
)
if (missing.length) process.stderr.write(`licenses: no license text found for ${missing.length}:\n  ${missing.join('\n  ')}\n`)
