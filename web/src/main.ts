// Fonts are bundled locally – no external font requests.
import '@fontsource-variable/inter'
import '@fontsource-variable/jetbrains-mono'
import '@fontsource-variable/source-serif-4'
import './styles/tokens.css'
import './styles/base.css'
import './styles/editor.css'
import './styles/admin.css'

import { mount } from 'svelte'
import App from './App.svelte'
// Applies theme and palette before anything renders, the sign-in page included.
import './lib/state/settings.svelte'

const target = document.getElementById('app')
if (!target) throw new Error('element #app is missing')

mount(App, { target })
