// Compile-and-render for single-file components, so a test can assert on what a
// component actually puts on the page.
//
// Nothing else in test/ mounts an SFC: the suite runs under bare `node --test`,
// and the tests that care about `.vue` files scan them as source text
// (errorRender.test.js). That convention cannot catch a rendering regression —
// the same objection a review raised against the hand transplant in
// jurisdictionPicker.test.js — so this compiles the real file instead.
//
// Two deliberate constraints, both from review of #281:
//
//  1. No new dependency. `vue` itself depends on @vue/compiler-sfc and exposes
//     it as the public subpath `vue/compiler-sfc`, so the compiler arrives with
//     the framework at exactly the framework's version — nothing to declare and
//     nothing to keep in step. Importing the bare package would have meant
//     relying on a hoisted transitive of @vitejs/plugin-vue.
//  2. No SSR. The app has no server-rendered path, so a component verified
//     through `renderToString` would be verified through a code path that never
//     runs in production — different compiler output, different runtime. This
//     mounts into jsdom with `createApp().mount()`, which is what main.js does.
//
// `node --test` discovers everything under test/, so this file is also executed
// as a test file with no tests in it — which passes, and is why nothing here may
// throw at module scope.
import { readFileSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
import { parse, compileScript } from 'vue/compiler-sfc'
import { JSDOM } from 'jsdom'

// vue/runtime-dom captures `document` ONCE, at module evaluation, into the node
// operations it hands the renderer. A DOM installed later leaves it holding null
// and every mount dies on `createTextNode` of null — so this runs as an import
// side effect, and vue is imported dynamically further down.
//
// The consequence for callers: **import this helper before anything that pulls
// in vue**, `src/i18n.js` included. ESM evaluates static imports in the order
// they are written, so the helper's import line has to come first in the test
// file. Nothing enforces that, which is why the failure mode is spelled out
// here: a null-document TypeError from inside the renderer means an import got
// reordered.
const dom = new JSDOM('<!doctype html><html><body></body></html>')
for (const key of ['window', 'document', 'Node', 'Element', 'HTMLElement', 'SVGElement', 'Text', 'Comment', 'DocumentFragment', 'CustomEvent']) {
  globalThis[key] = key === 'window' ? dom.window : dom.window[key]
}

// A `data:` module resolves nothing on its own: relative specifiers have no base
// to resolve against, and bare ones ("vue") are rejected outright because the
// URL scheme is not hierarchical. So every specifier is rewritten to an absolute
// URL first — relative ones against the SFC's own path, bare ones through Node's
// resolver, which finds web/node_modules from here.
//
// The alternative is writing the compiled file next to its source so Node
// resolves it normally, but `node --test` runs each file in its own process and
// a process that dies mid-run would leave that file behind under src/.
function absolutizeImports(code, sfcPath) {
  const base = pathToFileURL(sfcPath)
  return code.replaceAll(/(\bfrom\s*|\bimport\s*\(\s*)(['"])([^'"]+)\2/g, (_m, head, q, spec) => {
    const href = spec.startsWith('.') ? new URL(spec, base).href : import.meta.resolve(spec)
    return `${head}${q}${href}${q}`
  })
}

// Returns the component's default export, compiled the way Vite compiles it for
// the browser.
export async function compileSfc(sfcPath) {
  const source = readFileSync(sfcPath, 'utf8')
  const { descriptor, errors } = parse(source, { filename: sfcPath })
  if (errors.length) throw new Error(`parsing ${sfcPath}: ${errors[0].message}`)
  const { content } = compileScript(descriptor, { id: sfcPath, inlineTemplate: true })
  const mod = `data:text/javascript,${encodeURIComponent(absolutizeImports(content, sfcPath))}`
  return (await import(mod)).default
}

// Mounts the component and returns the host element's HTML. Async only because
// vue is imported dynamically, after the DOM above is in place.
export async function renderSfc(component, props = {}, slots = {}) {
  const { createApp, h } = await import('vue')
  const { i18n } = await import('../../src/i18n.js')
  // Mounted through a wrapper: `createApp` takes props but not slots, and a slot
  // is part of what this component's contract covers.
  const app = createApp({ render: () => h(component, props, slots) })
  app.use(i18n)
  const host = document.createElement('div')
  document.body.appendChild(host)
  app.mount(host)
  const html = host.innerHTML
  app.unmount()
  host.remove()
  return html
}

// A `v-if` that renders nothing leaves a comment placeholder behind, and a
// component's own template comments survive too, so "rendered nothing" has to be
// asked without them.
export function stripComments(html) {
  return html.replaceAll(/<!--[\s\S]*?-->/g, '').trim()
}
