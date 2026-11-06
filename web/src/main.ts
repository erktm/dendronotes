// Fonts are bundled locally – no external font requests.
import '@fontsource-variable/inter'
import '@fontsource-variable/jetbrains-mono'
import '@fontsource-variable/source-serif-4'
import './styles/tokens.css'
import './styles/base.css'
import './styles/editor.css'

import { mount } from 'svelte'
import App from './App.svelte'

const target = document.getElementById('app')
if (!target) throw new Error('element #app is missing')

mount(App, { target })
