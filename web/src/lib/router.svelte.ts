import { encodePath } from './paths'

// URLs:
//   /                      the last soma (chosen by the app)
//   /s/<soma>              a soma without an open note
//   /s/<soma>/n/<path>     a note
//   /s/<soma>/graph        the graph
//   /s/<soma>/l/<id>       an internal link: opens the entry wherever it is now
//   /admin, /admin/<tab>   the admin area
//   /tutorials, /tutorials/<topic>  the tutorials
// The server answers every such URL with index.html, so links and reloads work.

export type View = 'soma' | 'graph' | 'admin' | 'tutorials'

export interface Route {
  soma: string | null
  notePath: string | null
  view: View
  adminTab: string
  /** An internal link to follow (/s/<soma>/l/<id>). */
  link?: string
  /** The open tutorial (/tutorials/<topic>). */
  topic?: string
}

const SOMA = /^\/s\/([a-z0-9]{1,40})(?:\/(n\/.+|graph|l\/[a-z0-9]{1,40}))?\/?$/

export function parse(pathname: string): Route {
  if (pathname === '/tutorials' || pathname.startsWith('/tutorials/')) {
    return { soma: null, notePath: null, view: 'tutorials', adminTab: '', topic: pathname.slice('/tutorials/'.length) }
  }
  if (pathname === '/admin' || pathname.startsWith('/admin/')) {
    return { soma: null, notePath: null, view: 'admin', adminTab: pathname.slice('/admin/'.length) || 'users' }
  }
  const m = SOMA.exec(pathname)
  if (!m) return { soma: null, notePath: null, view: 'soma', adminTab: '' }
  const soma = m[1]!
  const rest = m[2]
  if (rest === 'graph') return { soma, notePath: null, view: 'graph', adminTab: '' }
  if (rest?.startsWith('l/')) return { soma, notePath: null, view: 'soma', adminTab: '', link: rest.slice(2) }
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
  if (r.view === 'tutorials') return r.topic ? `/tutorials/${r.topic}` : '/tutorials'
  if (!r.soma) return '/'
  if (r.view === 'graph') return `/s/${r.soma}/graph`
  if (r.link) return `/s/${r.soma}/l/${r.link}`
  return r.notePath ? noteURL(r.soma, r.notePath) : `/s/${r.soma}`
}

class Router {
  soma = $state<string | null>(null)
  notePath = $state<string | null>(null)
  view = $state<View>('soma')
  adminTab = $state('users')
  /** An internal link the app follows, then replaces with the entry's URL. */
  link = $state<string | null>(null)
  /** The open tutorial. */
  topic = $state('')

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
    this.link = r.link ?? null
    this.topic = r.topic ?? ''
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

  /** Opens the tutorials (a topic, or the first one). */
  goTutorials(topic = ''): void {
    this.push({ soma: this.soma, notePath: null, view: 'tutorials', adminTab: '', topic }, false)
  }

  goAdmin(tab = 'users'): void {
    this.push({ soma: this.soma, notePath: null, view: 'admin', adminTab: tab }, false)
  }
}

export const router = new Router()
