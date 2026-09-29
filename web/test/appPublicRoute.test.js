// Regression coverage for PR #372 review finding F1 (App.vue's isPublicRoute
// switch from a hardcoded path list to route.meta.public broke the initial
// page load) and the companion nit F3 (the route.path watcher's own
// hardcoded public-path list, replaced by a router.afterEach keyed on the
// same isRoutePublic logic as F1's fix).
//
// isRoutePublic and the afterEach reload decision are transplanted verbatim
// from App.vue — same caveat as router.test.js and loginOrgSlug.test.js:
// this is a COPY, not an import (App.vue is a .vue SFC with DOM/composable
// dependencies this harness doesn't load under `node --test`), so it pins
// the intended logic rather than enforcing it against the shipped file.
import test from 'node:test'
import assert from 'node:assert/strict'

// Transplant of App.vue's isRoutePublic.
function isRoutePublic(r) {
  return !!r.meta.public || (r.path === '/' && r.matched.length === 0)
}

// Transplant of the router.afterEach reload condition.
function shouldReloadOnTransition(from, to) {
  return isRoutePublic(from) && !isRoutePublic(to)
}

const START_LOCATION = { path: '/', meta: {}, matched: [] }
const landing = { path: '/', meta: { public: true }, matched: [{}] }
const orgLogin = { path: '/acme/login', meta: { public: true }, matched: [{}] }
const bareLogin = { path: '/login', meta: { public: true }, matched: [{}] }
const overview = { path: '/acme/overview', meta: {}, matched: [{}] }

test('vue-router START_LOCATION (path /, nothing matched yet) counts as public', () => {
  // This is the state App and its onMounted see on the very first render,
  // before the initial navigation resolves (main.js mounts without waiting
  // on router.isReady()). Without this clause, mount-time loadAppData() runs
  // for every visitor, token or not, reproducing the redirect-loop bug this
  // PR set out to fix.
  assert.equal(isRoutePublic(START_LOCATION), true)
})

test('a real / (Landing, fully matched) is also public', () => {
  assert.equal(isRoutePublic(landing), true)
})

test('org-scoped /<org>/login is public via meta.public, not a hardcoded path', () => {
  assert.equal(isRoutePublic(orgLogin), true)
})

test('a non-public route is not public', () => {
  assert.equal(isRoutePublic(overview), false)
})

test('reload fires on the apex-placeholder-to-overview hop', () => {
  // The transition the old hardcoded watcher's `/` entry existed for:
  // mount-time loadAppData() was skipped (isPublicRoute true at the
  // placeholder), so something has to fire it once the router lands on the
  // real, non-public destination.
  assert.equal(shouldReloadOnTransition(START_LOCATION, overview), true)
})

test('reload does NOT fire landing directly on a public page from the placeholder', () => {
  // Landing on /acme/login or bare /login needs no app data — firing
  // loadAppData here is exactly what reproduces the 401-driven redirect loop.
  assert.equal(shouldReloadOnTransition(START_LOCATION, orgLogin), false)
  assert.equal(shouldReloadOnTransition(START_LOCATION, bareLogin), false)
})

test('reload fires on login success (public login page to non-public overview)', () => {
  // Review finding F3: this used to work only by accident, via the org-param
  // branch of the route.path watcher matching because currentUserData was
  // still null. The afterEach hook now covers it explicitly.
  assert.equal(shouldReloadOnTransition(orgLogin, overview), true)
})

test('reload does not fire between two non-public routes', () => {
  const otherOverview = { path: '/beta/overview', meta: {}, matched: [{}] }
  assert.equal(shouldReloadOnTransition(overview, otherOverview), false)
})
