import { encodePath } from './paths'

// URLs: "/" shows no note, "/n/<path>" opens a note, "/graph" the graph.
// The server answers every such URL with index.html, so links and reloads work.
const PREFIX = '/n/'
const GRAPH = '/graph'

function parse(pathname: string): string | null {
  if (!pathname.startsWith(PREFIX)) return null
  try {
    const p = pathname.slice(PREFIX.length).split('/').map(decodeURIComponent).join('/')
    return p || null
  } catch {
    return null
  }
}

class Router {
  notePath = $state<string | null>(parse(location.pathname))
  /** The graph view is open (instead of a note). */
  graph = $state(location.pathname === GRAPH)

  constructor() {
    window.addEventListener('popstate', () => {
      this.notePath = parse(location.pathname)
      this.graph = location.pathname === GRAPH
    })
  }

  go(path: string | null, replace = false): void {
    this.push(path ? PREFIX + encodePath(path) : '/', replace)
    this.notePath = path
    this.graph = false
  }

  goGraph(): void {
    this.push(GRAPH, false)
    this.notePath = null
    this.graph = true
  }

  private push(url: string, replace: boolean): void {
    if (url === location.pathname) return
    if (replace) history.replaceState(null, '', url)
    else history.pushState(null, '', url)
  }
}

export const router = new Router()
