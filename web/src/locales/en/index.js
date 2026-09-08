// The `en` message bundle: one file per area, merged into a single nested
// keyspace. Area = the top-level key, and it is also the filename — so
// `risks.table.header.likelihood` will live in `risks.json` and nowhere else.
//
// The split is not cosmetic. Extraction is one PR per view, and a single
// en.json would make ~21 concurrent PRs collide on one file. Each area file
// therefore arrives with the PR that extracts that area: one new file plus one
// import line here. Only `common.json` exists up front, because the shared
// vocabulary is what every area draws on.
//
// Import attributes (`with { type: 'json' }`) are required by Node for JSON
// modules, and understood by Vite/Rollup — the same file therefore loads both
// in the bundler and under `node --test`.
import admin from './admin.json' with { type: 'json' }
import assets from './assets.json' with { type: 'json' }
import audit from './audit.json' with { type: 'json' }
import auth from './auth.json' with { type: 'json' }
import changes from './changes.json' with { type: 'json' }
import common from './common.json' with { type: 'json' }
import correctiveActions from './corrective_actions.json' with { type: 'json' }
import dashboard from './dashboard.json' with { type: 'json' }
import components from './components.json' with { type: 'json' }
import incidents from './incidents.json' with { type: 'json' }
import landing from './landing.json' with { type: 'json' }
import legal from './legal.json' with { type: 'json' }
import notifications from './notifications.json' with { type: 'json' }
import objectives from './objectives.json' with { type: 'json' }
import organizations from './organizations.json' with { type: 'json' }
import programs from './programs.json' with { type: 'json' }
import reviews from './reviews.json' with { type: 'json' }
import risks from './risks.json' with { type: 'json' }
import settings from './settings.json' with { type: 'json' }
import shell from './shell.json' with { type: 'json' }
import suppliers from './suppliers.json' with { type: 'json' }
import systems from './systems.json' with { type: 'json' }
import tasks from './tasks.json' with { type: 'json' }

export default {
  admin,
  assets,
  audit,
  auth,
  changes,
  common,
  corrective_actions: correctiveActions,
  components,
  dashboard,
  incidents,
  landing,
  legal,
  notifications,
  objectives,
  organizations,
  programs,
  reviews,
  risks,
  settings,
  shell,
  suppliers,
  systems,
  tasks,
}
