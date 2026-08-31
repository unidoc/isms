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
import common from './common.json' with { type: 'json' }
import notifications from './notifications.json' with { type: 'json' }

export default {
  common,
  notifications,
}
