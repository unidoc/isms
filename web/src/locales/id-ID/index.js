// The `id-ID` message bundle: one file per area, merged into a single nested
// keyspace. Area = the top-level key, and it is also the filename — so
// `risks.table.header.likelihood` will live in `risks.json` and nowhere else.
//
// This directory mirrors `en/` file for file and key for key. Only the values
// are translated. A key missing here is not an error: `fallbackLocale` renders
// it in English, so a lagging translation degrades gracefully rather than
// showing a raw key — which is also why an area file only appears here once the
// matching `en/` one exists.
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
import components from './components.json' with { type: 'json' }
import corrective_actions from './corrective_actions.json' with { type: 'json' }
import dashboard from './dashboard.json' with { type: 'json' }
import documents from './documents.json' with { type: 'json' }
import inbox from './inbox.json' with { type: 'json' }
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
  components,
  corrective_actions,
  dashboard,
  documents,
  inbox,
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
