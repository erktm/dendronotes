import { encodePath } from './paths'

// URLs: "/" shows no note, "/n/<path>" opens a note. The server answers
// every such URL with index.html, so links and reloads work.
const PREFIX = '/n/'

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

  constructor() {
    window.addEventListener('popstate', () => {
      this.notePath = parse(location.pathname)
    })
  }

  go(path: string | null, replace = false): void {
    const url = path ? PREFIX + encodePath(path) : '/'
    if (url !== location.pathname) {
      if (replace) history.replaceState(null, '', url)
      else history.pushState(null, '', url)
    }
    this.notePath = path
  }
}

export const router = new Router()
