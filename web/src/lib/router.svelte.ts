import { encodePath } from './paths'

// URLs:
//   /                      the last soma (chosen by the app)
//   /s/<soma>              a soma without an open note
//   /s/<soma>/n/<path>     a note
//   /s/<soma>/graph        the graph
//   /admin, /admin/<tab>   the admin area
// The server answers every such URL with index.html, so links and reloads work.

export type View = 'soma' | 'graph' | 'admin'

export interface Route {
  soma: string | null
  notePath: string | null
  view: View
  adminTab: string
}

const SOMA = /^\/s\/([a-z0-9]{1,40})(?:\/(n\/.+|graph))?\/?$/

export function parse(pathname: string): Route {
  if (pathname === '/admin' || pathname.startsWith('/admin/')) {
    return { soma: null, notePath: null, view: 'admin', adminTab: pathname.slice('/admin/'.length) || 'users' }
  }
  const m = SOMA.exec(pathname)
  if (!m) return { soma: null, notePath: null, view: 'soma', adminTab: '' }
  const soma = m[1]!
  const rest = m[2]
  if (rest === 'graph') return { soma, notePath: null, view: 'graph', adminTab: '' }
  if (rest?.startsWith('n/')) {
    try {
      const p = rest.slice(2).split('/').map(decodeURIComponent).join('/')
      return { soma, notePath: p || null, view: 'soma', adminTab: '' }
    } catch {
      return { soma, notePath: null, view: 'soma', adminTab: '' }
    }
  }
  return { soma, notePath: null, view: 'soma', adminTab: '' }
}

/** The URL of a note in a soma. */
export function noteURL(soma: string, path: string): string {
  return `/s/${soma}/n/${encodePath(path)}`
}

function format(r: Route): string {
  if (r.view === 'admin') return r.adminTab && r.adminTab !== 'users' ? `/admin/${r.adminTab}` : '/admin'
  if (!r.soma) return '/'
  if (r.view === 'graph') return `/s/${r.soma}/graph`
  return r.notePath ? noteURL(r.soma, r.notePath) : `/s/${r.soma}`
}

class Router {
  soma = $state<string | null>(null)
  notePath = $state<string | null>(null)
  view = $state<View>('soma')
  adminTab = $state('users')

  /** The graph view is open (instead of a note). */
  graph = $derived(this.view === 'graph')

  constructor() {
    this.apply(parse(location.pathname))
    window.addEventListener('popstate', () => this.apply(parse(location.pathname)))
  }

  private apply(r: Route): void {
    this.soma = r.soma
    this.notePath = r.notePath
    this.view = r.view
    this.adminTab = r.adminTab || 'users'
  }

  private push(r: Route, replace: boolean): void {
    const url = format(r)
    if (url !== location.pathname) {
      if (replace) history.replaceState(null, '', url)
      else history.pushState(null, '', url)
    }
    this.apply(r)
  }

  /** Opens a note (or none) in the current soma. */
  go(path: string | null, replace = false): void {
    this.push({ soma: this.soma, notePath: path, view: 'soma', adminTab: '' }, replace)
  }

  /** Opens a note in any soma. */
  goNote(soma: string, path: string): void {
    this.push({ soma, notePath: path, view: 'soma', adminTab: '' }, false)
  }

  /** Switches to another soma (without a note). */
  goSoma(soma: string, replace = false): void {
    this.push({ soma, notePath: null, view: 'soma', adminTab: '' }, replace)
  }

  goGraph(): void {
    this.push({ soma: this.soma, notePath: null, view: 'graph', adminTab: '' }, false)
  }

  goAdmin(tab = 'users'): void {
    this.push({ soma: this.soma, notePath: null, view: 'admin', adminTab: tab }, false)
  }
}

export const router = new Router()
