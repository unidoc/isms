<template>
  <!-- The public trust center (#519). A light page whatever the app theme is: it
       is read by visitors with no account, often from a link in a security
       questionnaire. Everything it shows comes from the anonymous trust API, with
       no Authorization header, so a signed-in reader sees what a visitor sees. -->
  <div class="trust-page min-h-screen bg-slate-50 text-slate-900 flex flex-col">
    <div v-if="state === 'loading'" class="flex-1 flex items-center justify-center text-sm text-slate-500" data-trust="loading">
      {{ $t('trust.public.loading') }}
    </div>

    <div v-else-if="state === 'unavailable'" class="flex-1 flex items-center justify-center px-4" data-trust="unavailable">
      <p class="text-slate-600 text-center">{{ $t('trust.public.unavailable') }}</p>
    </div>

    <div v-else-if="state === 'error'" class="flex-1 flex flex-col items-center justify-center gap-3 px-4" data-trust="error">
      <p class="text-slate-600 text-center">{{ $t('trust.public.error_load') }}</p>
      <button type="button" class="px-4 py-2 rounded-lg text-sm font-medium text-white hover:opacity-90 trust-fill" data-trust="retry" @click="load">
        {{ $t('trust.public.retry') }}
      </button>
    </div>

    <template v-else>
      <header class="bg-white border-b border-slate-200" data-trust="page">
        <div class="max-w-4xl mx-auto px-4 py-4 flex items-center gap-3">
          <img v-if="!logoFailed" :src="logoSrc" alt="" class="h-9 w-auto max-w-[8rem] object-contain" @error="logoFailed = true" />
          <div class="min-w-0 flex-1">
            <div class="font-semibold truncate trust-brand-text" data-trust="brand">{{ ov.branding_name }}</div>
            <div class="text-xs text-slate-500">{{ $t('trust.public.heading') }}</div>
          </div>
          <template v-if="signInShown">
            <div v-if="visitor" class="flex items-center gap-2 text-sm shrink-0" data-trust="visitor">
              <span class="text-slate-600 max-w-[14rem] truncate" data-trust="visitor-email">{{ $t('trust.public.signed_in_as', { email: visitor.email }) }}</span>
              <button type="button" class="font-medium trust-brand-text hover:underline" data-trust="sign-out" :disabled="signingOut" @click="signOut">
                {{ $t('trust.public.sign_out') }}
              </button>
            </div>
            <button v-else type="button" class="text-sm font-medium trust-brand-text hover:underline shrink-0 text-right disabled:opacity-70 disabled:no-underline disabled:cursor-default" data-trust="sign-in-link" :disabled="!visitorFeatures" @click="openSignIn">
              {{ $t('trust.public.sign_in_link') }}
            </button>
          </template>
          <a v-if="hasSession" href="/" class="text-sm font-medium trust-brand-text hover:underline shrink-0" data-trust="back-to-app" @click="go($event, '/')">
            {{ $t('trust.public.back_to_app') }}
          </a>
        </div>

        <!-- Sign-in form: a one-time link by email. The answer to a send is the
             same text whatever the server knows about the address. -->
        <div v-if="signInAvailable && !visitor && formOpen" class="border-t border-slate-100 bg-slate-50" data-trust="sign-in-form">
          <form class="max-w-4xl mx-auto px-4 py-4 space-y-3" novalidate @submit.prevent="sendLink">
            <h2 class="text-sm font-semibold">{{ $t('trust.public.sign_in_heading') }}</h2>
            <p class="text-sm text-slate-600">{{ $t('trust.public.sign_in_intro') }}</p>
            <div>
              <label for="trust-sign-in-email" class="block text-sm font-medium text-slate-700">{{ $t('trust.public.sign_in_email') }}</label>
              <input id="trust-sign-in-email" v-model="email" type="email" autocomplete="email" required maxlength="320"
                class="mt-1 w-full max-w-md rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900" data-trust="sign-in-email" />
            </div>
            <p v-if="privacyUrl" class="text-xs text-slate-500">
              <a :href="privacyUrl" target="_blank" rel="noopener noreferrer" class="underline hover:text-slate-700" data-trust="privacy">{{ $t('trust.public.sign_in_privacy') }}</a>
            </p>
            <p v-if="sendState === 'sent'" class="text-sm text-slate-700" role="status" data-trust="sign-in-sent">{{ $t('trust.public.sign_in_sent') }}</p>
            <p v-else-if="sendError" class="text-sm text-red-700" role="alert" data-trust="sign-in-error">{{ sendError }}</p>
            <div class="flex items-center gap-2">
              <button type="submit" class="px-4 py-2 rounded-lg text-sm font-medium text-white hover:opacity-90 trust-fill disabled:opacity-60" data-trust="sign-in-submit" :disabled="sendState === 'sending'">
                {{ sendLabel }}
              </button>
              <button type="button" class="px-3 py-2 rounded-lg text-sm text-slate-700 hover:bg-slate-100" data-trust="sign-in-cancel" @click="formOpen = false">
                {{ $t('trust.public.sign_in_cancel') }}
              </button>
            </div>
          </form>
        </div>
      </header>

      <main class="flex-1 w-full max-w-4xl mx-auto px-4 py-8 space-y-8">
        <!-- Document page -->
        <section v-if="docId" data-trust="document-page">
          <a :href="basePath" class="text-sm font-medium trust-brand-text hover:underline" data-trust="doc-back" @click="go($event, basePath)">
            <svg class="inline w-4 h-4 -mt-0.5" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5M11 6l-6 6 6 6" /></svg>
            {{ $t('trust.public.doc_back') }}
          </a>

          <p v-if="docState === 'loading'" class="mt-6 text-sm text-slate-500">{{ $t('trust.public.loading') }}</p>

          <p v-else-if="docState === 'missing'" class="mt-6 text-slate-600" data-trust="doc-unavailable">
            {{ $t('trust.public.doc_unavailable') }}
          </p>

          <div v-else-if="docState === 'error'" class="mt-6 flex flex-col items-start gap-3">
            <p class="text-slate-600">{{ $t('trust.public.error_load') }}</p>
            <button type="button" class="px-4 py-2 rounded-lg text-sm font-medium text-white hover:opacity-90 trust-fill" @click="loadDocument">
              {{ $t('trust.public.retry') }}
            </button>
          </div>

          <div v-else-if="docState === 'locked'" class="mt-6 bg-white border border-slate-200 rounded-xl p-6" data-trust="locked">
            <h1 class="text-2xl font-semibold">{{ lockedItem.title }}</h1>
            <p class="mt-2 text-sm text-slate-600">{{ $t('trust.public.doc_locked', { access: accessLabel(lockedItem.access_level) }) }}</p>
            <p v-if="needsSignIn(lockedItem)" class="mt-4 text-sm">
              <button type="button" class="font-medium trust-brand-text hover:underline disabled:opacity-70 disabled:no-underline disabled:cursor-default" data-trust="doc-sign-in" :disabled="!visitorFeatures" @click="openSignIn">{{ $t('trust.public.doc_sign_in') }}</button>
            </p>
            <p v-else-if="needsNDA(lockedItem)" class="mt-4 text-sm">
              <button type="button" class="font-medium trust-brand-text hover:underline" data-trust="doc-accept-nda" @click="openNDA">{{ $t('trust.public.action_accept_nda') }}</button>
            </p>
            <p v-else-if="ndaLocked(lockedItem)" class="mt-4 text-sm text-slate-600" data-trust="doc-nda-unavailable">{{ $t('trust.public.action_nda_unavailable') }}</p>
            <p v-else class="mt-4 text-sm">
              <a v-if="isEmail(ov.security_contact)" :href="'mailto:' + ov.security_contact" class="font-medium trust-brand-text hover:underline">{{ $t('trust.public.action_contact', { contact: ov.security_contact }) }}</a>
              <span v-else-if="ov.security_contact" class="font-medium">{{ $t('trust.public.action_contact', { contact: ov.security_contact }) }}</span>
              <span v-else class="text-slate-600">{{ $t('trust.public.action_on_request') }}</span>
            </p>
          </div>

          <article v-else-if="docState === 'ready'" class="mt-6" data-trust="doc">
            <h1 class="text-2xl font-semibold">{{ doc.title }}</h1>
            <p class="mt-2 text-sm text-slate-600 flex flex-wrap gap-x-4 gap-y-1">
              <span>{{ $t('trust.public.doc_version', { version: doc.version }) }}</span>
              <span>{{ $t('trust.public.doc_approved', { date: fmtDate(doc.approved_at) }) }}</span>
              <span class="min-w-0 truncate" :title="doc.content_hash">{{ $t('trust.public.doc_hash') }}: <code class="text-xs">{{ shortHash(doc.content_hash) }}</code></span>
            </p>
            <div class="trust-prose mt-6" data-trust="doc-body" @click="onContentClick" v-html="docHtml"></div>
          </article>
        </section>

        <!-- Landing page -->
        <template v-else>
          <p v-if="!hasOverview && !documents.length && !certifications.length && !subprocessors.length" class="text-slate-600" data-trust="empty">
            {{ $t('trust.public.empty') }}
          </p>

          <section v-if="hasOverview" data-trust="overview">
            <div class="trust-prose" @click="onContentClick" v-html="overviewHtml"></div>
            <p v-if="ov.security_contact" class="mt-4 text-sm text-slate-600">
              {{ $t('trust.public.security_contact') }}:
              <a v-if="isEmail(ov.security_contact)" :href="'mailto:' + ov.security_contact" class="font-medium trust-brand-text hover:underline">{{ ov.security_contact }}</a>
              <span v-else class="font-medium text-slate-900">{{ ov.security_contact }}</span>
            </p>
          </section>

          <p v-if="ndaUnavailableNote" class="text-sm text-amber-800 bg-amber-50 border border-amber-200 rounded-lg px-3 py-2" role="status" data-trust="nda-unavailable-note">{{ $t('trust.public.nda_unavailable_note') }}</p>

          <section v-if="certifications.length" data-trust="certifications">
            <h2 class="text-lg font-semibold">{{ $t('trust.public.certifications.heading') }}</h2>
            <div class="mt-4 bg-white border border-slate-200 rounded-xl overflow-hidden" role="table">
              <div class="hidden md:grid md:grid-cols-[minmax(0,2fr)_minmax(0,1.2fr)_minmax(0,1.6fr)_minmax(0,1.1fr)_minmax(0,1.4fr)] gap-4 px-4 py-2 text-xs font-medium uppercase tracking-wide text-slate-500 border-b border-slate-200 bg-slate-50" role="row">
                <div role="columnheader">{{ $t('trust.public.certifications.col_name') }}</div>
                <div role="columnheader">{{ $t('trust.public.certifications.col_type') }}</div>
                <div role="columnheader">{{ $t('trust.public.certifications.col_issuer') }}</div>
                <div role="columnheader">{{ $t('trust.public.certifications.col_status') }}</div>
                <div role="columnheader">{{ $t('trust.public.certifications.col_action') }}</div>
              </div>
              <div v-for="c in certifications" :key="c.id" role="row" data-trust="cert-row"
                class="grid gap-1 md:gap-4 md:grid-cols-[minmax(0,2fr)_minmax(0,1.2fr)_minmax(0,1.6fr)_minmax(0,1.1fr)_minmax(0,1.4fr)] px-4 py-3 border-b border-slate-100 last:border-b-0 md:items-center">
                <div role="cell" class="font-medium break-words" data-trust="cert-title">{{ c.title }}</div>
                <div role="cell" class="text-sm text-slate-600" data-trust="cert-type">{{ certTypeLabel(c.link_type) }}</div>
                <div role="cell" class="text-sm text-slate-600 min-w-0 break-words">
                  <div v-if="c.issuer" data-trust="cert-issuer">{{ c.issuer }}</div>
                  <div v-if="c.valid_until" class="text-xs text-slate-500" data-trust="cert-valid-until">{{ $t('trust.public.certifications.valid_until', { date: fmtDay(c.valid_until) }) }}</div>
                </div>
                <div role="cell">
                  <span v-if="c.state" class="inline-block text-xs font-medium px-2 py-0.5 rounded-full" :class="certStateClass(c.state)" data-trust="cert-state">{{ certStateLabel(c.state) }}</span>
                </div>
                <div role="cell" class="text-sm min-w-0 break-words">
                  <template v-if="mayRead(c) && c.source === 'file'">
                    <button v-if="source && source.certificationFile" type="button" class="font-medium trust-brand-text hover:underline" data-trust="cert-download" @click="downloadCert(c)">{{ $t('trust.public.certifications.action_download') }}</button>
                    <a v-else :href="certFileHref(c)" download class="font-medium trust-brand-text hover:underline" data-trust="cert-download" @click="downloadGated($event, c)">{{ $t('trust.public.certifications.action_download') }}</a>
                  </template>
                  <a v-else-if="mayRead(c) && c.source === 'link' && safeHref(c.url)" :href="safeHref(c.url)" target="_blank" rel="noopener noreferrer" class="font-medium trust-brand-text hover:underline" data-trust="cert-view">{{ $t('trust.public.certifications.action_view_at', { host: hostOf(c.url) }) }}</a>
                  <button v-else-if="needsSignIn(c)" type="button" class="font-medium trust-brand-text hover:underline disabled:opacity-70 disabled:no-underline disabled:cursor-default" data-trust="cert-sign-in" :disabled="!visitorFeatures" @click="openSignIn">{{ signInLabel }}</button>
                  <button v-else-if="needsNDA(c)" type="button" class="font-medium trust-brand-text hover:underline" data-trust="cert-accept-nda" @click="openNDA">{{ $t('trust.public.action_accept_nda') }}</button>
                  <span v-else-if="ndaLocked(c)" class="text-slate-500" data-trust="cert-nda-unavailable">{{ $t('trust.public.action_nda_unavailable') }}</span>
                  <a v-else-if="isEmail(ov.security_contact)" :href="'mailto:' + ov.security_contact" class="font-medium trust-brand-text hover:underline" data-trust="contact">{{ $t('trust.public.action_contact', { contact: ov.security_contact }) }}</a>
                  <span v-else-if="ov.security_contact" class="text-slate-700" data-trust="contact">{{ $t('trust.public.action_contact', { contact: ov.security_contact }) }}</span>
                  <span v-else class="text-slate-500" data-trust="on-request">{{ $t('trust.public.action_on_request') }}</span>
                </div>
              </div>
            </div>
          </section>

          <section v-if="documents.length" data-trust="documents">
            <h2 class="text-lg font-semibold">{{ $t('trust.public.documents') }}</h2>

            <div class="mt-3 flex flex-wrap gap-2" role="group" :aria-label="$t('trust.public.filter_label')">
              <button v-for="f in filters" :key="f.key" type="button" :aria-pressed="filter === f.key"
                class="px-3 py-1 rounded-full text-sm border transition-colors"
                :class="filter === f.key ? 'text-white border-transparent trust-fill' : 'bg-white text-slate-700 border-slate-300 hover:border-slate-400'"
                :data-trust-filter="f.key" @click="filter = f.key">
                {{ f.label }}
              </button>
            </div>

            <div class="mt-4 bg-white border border-slate-200 rounded-xl overflow-hidden" role="table">
              <div class="hidden md:grid md:grid-cols-[minmax(0,2fr)_minmax(0,1.4fr)_minmax(0,1.2fr)_minmax(0,1.4fr)] gap-4 px-4 py-2 text-xs font-medium uppercase tracking-wide text-slate-500 border-b border-slate-200 bg-slate-50" role="row">
                <div role="columnheader">{{ $t('trust.public.col_document') }}</div>
                <div role="columnheader">{{ $t('trust.public.col_version') }}</div>
                <div role="columnheader">{{ $t('trust.public.col_access') }}</div>
                <div role="columnheader">{{ $t('trust.public.col_action') }}</div>
              </div>
              <p v-if="!visibleDocuments.length" class="px-4 py-6 text-sm text-slate-500">{{ $t('trust.public.empty_filter') }}</p>
              <div v-for="d in visibleDocuments" :key="d.id" role="row" data-trust="row"
                class="grid gap-1 md:gap-4 md:grid-cols-[minmax(0,2fr)_minmax(0,1.4fr)_minmax(0,1.2fr)_minmax(0,1.4fr)] px-4 py-3 border-b border-slate-100 last:border-b-0 md:items-center">
                <div role="cell" class="font-medium break-words">{{ d.title }}</div>
                <div role="cell" class="text-sm text-slate-600 min-w-0">
                  <span v-if="d.version">v{{ d.version }}</span>
                  <code v-if="d.content_hash" class="ml-2 text-xs text-slate-500" :title="d.content_hash">{{ shortHash(d.content_hash) }}</code>
                </div>
                <div role="cell" class="text-sm text-slate-700 flex items-center gap-1.5">
                  <svg v-if="d.access_level === 'public'" class="w-4 h-4 shrink-0" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9" /><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18" /></svg>
                  <svg v-else-if="d.access_level === 'gated'" class="w-4 h-4 shrink-0" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="5" y="11" width="14" height="9" rx="2" /><path d="M8 11V8a4 4 0 0 1 8 0v3" /></svg>
                  <svg v-else class="w-4 h-4 shrink-0" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="8" cy="15" r="4" /><path d="M11 12l9-9M16 7l3 3" /></svg>
                  <span>{{ accessLabel(d.access_level) }}</span>
                </div>
                <div role="cell" class="text-sm min-w-0 break-words">
                  <a v-if="mayRead(d)" :href="basePath + '/documents/' + d.id" class="font-medium trust-brand-text hover:underline" data-trust="view"
                    @click="go($event, basePath + '/documents/' + d.id)">{{ $t('trust.public.action_view') }}</a>
                  <button v-else-if="needsSignIn(d)" type="button" class="font-medium trust-brand-text hover:underline disabled:opacity-70 disabled:no-underline disabled:cursor-default" data-trust="sign-in-to-view" :disabled="!visitorFeatures" @click="openSignIn">{{ signInLabel }}</button>
                  <button v-else-if="needsNDA(d)" type="button" class="font-medium trust-brand-text hover:underline" data-trust="accept-nda" @click="openNDA">{{ $t('trust.public.action_accept_nda') }}</button>
                  <span v-else-if="ndaLocked(d)" class="text-slate-500" data-trust="nda-unavailable">{{ $t('trust.public.action_nda_unavailable') }}</span>
                  <a v-else-if="isEmail(ov.security_contact)" :href="'mailto:' + ov.security_contact" class="font-medium trust-brand-text hover:underline" data-trust="contact">{{ $t('trust.public.action_contact', { contact: ov.security_contact }) }}</a>
                  <span v-else-if="ov.security_contact" class="text-slate-700" data-trust="contact">{{ $t('trust.public.action_contact', { contact: ov.security_contact }) }}</span>
                  <span v-else class="text-slate-500" data-trust="on-request">{{ $t('trust.public.action_on_request') }}</span>
                </div>
              </div>
            </div>
            <p class="mt-3 text-xs text-slate-500">{{ $t('trust.public.hash_note') }}</p>
          </section>

          <section v-if="subprocessors.length" data-trust="subprocessors">
            <h2 class="text-lg font-semibold">{{ $t('trust.public.subprocessors.heading') }}</h2>
            <p v-if="ov.subprocessors_changed_at" class="mt-1 text-xs text-slate-500" data-trust="subprocessors-updated">{{ $t('trust.public.subprocessors.updated', { date: fmtDate(ov.subprocessors_changed_at) }) }}</p>
            <div class="mt-4 bg-white border border-slate-200 rounded-xl overflow-hidden" role="table">
              <div class="hidden md:grid md:grid-cols-[minmax(0,1.5fr)_minmax(0,2fr)_minmax(0,1.3fr)_minmax(0,2fr)] gap-4 px-4 py-2 text-xs font-medium uppercase tracking-wide text-slate-500 border-b border-slate-200 bg-slate-50" role="row">
                <div role="columnheader">{{ $t('trust.public.subprocessors.col_name') }}</div>
                <div role="columnheader">{{ $t('trust.public.subprocessors.col_purpose') }}</div>
                <div role="columnheader">{{ $t('trust.public.subprocessors.col_location') }}</div>
                <div role="columnheader">{{ $t('trust.public.subprocessors.col_data') }}</div>
              </div>
              <div v-for="sp in subprocessors" :key="sp.id" role="row" data-trust="subprocessor-row"
                class="grid gap-2 md:gap-4 md:grid-cols-[minmax(0,1.5fr)_minmax(0,2fr)_minmax(0,1.3fr)_minmax(0,2fr)] px-4 py-3 border-b border-slate-100 last:border-b-0">
                <div role="cell" class="font-medium break-words" data-trust="sp-name">{{ sp.name }}</div>
                <div role="cell" class="text-sm text-slate-600 break-words">
                  <span class="md:hidden block text-[11px] uppercase tracking-wide text-slate-400">{{ $t('trust.public.subprocessors.col_purpose') }}</span>
                  <span data-trust="sp-purpose">{{ sp.purpose || '\u2014' }}</span>
                </div>
                <div role="cell" class="text-sm text-slate-600 break-words">
                  <span class="md:hidden block text-[11px] uppercase tracking-wide text-slate-400">{{ $t('trust.public.subprocessors.col_location') }}</span>
                  <span data-trust="sp-location">{{ sp.location || '\u2014' }}</span>
                </div>
                <div role="cell" class="text-sm text-slate-600 break-words">
                  <span class="md:hidden block text-[11px] uppercase tracking-wide text-slate-400">{{ $t('trust.public.subprocessors.col_data') }}</span>
                  <span data-trust="sp-data">{{ sp.data_categories || '\u2014' }}</span>
                </div>
              </div>
            </div>
          </section>
        </template>
      </main>

      <!-- The NDA panel: a signed-in visitor reads the current version and accepts
           it. Never in the manager's preview. -->
      <div v-if="ndaOpen" class="fixed inset-0 z-50 flex items-start justify-center px-4 py-8 overflow-y-auto" data-trust="nda-panel">
        <div class="fixed inset-0 bg-slate-900/50" @click="closeNDA" />
        <div class="relative bg-white text-slate-900 border border-slate-200 rounded-2xl shadow-2xl w-full max-w-2xl p-6 space-y-4" role="dialog" aria-modal="true" :aria-label="$t('trust.public.nda_heading')">
          <p v-if="ndaLoading" class="text-sm text-slate-500" data-trust="nda-loading">{{ $t('trust.public.nda_loading') }}</p>
          <p v-else-if="!ndaDoc" class="text-sm text-red-700" role="alert" data-trust="nda-load-error">{{ $t('trust.public.nda_load_error') }}</p>
          <template v-else>
            <div>
              <h2 class="text-xl font-semibold" data-trust="nda-title">{{ ndaDoc.title }}</h2>
              <p class="mt-1 text-sm text-slate-600" data-trust="nda-version">{{ $t('trust.public.doc_version', { version: ndaDoc.version }) }}</p>
            </div>
            <p v-if="ndaChanged" class="text-sm text-amber-800 bg-amber-50 border border-amber-200 rounded-lg px-3 py-2" role="alert" data-trust="nda-changed">{{ $t('trust.public.nda_changed') }}</p>
            <div class="trust-prose max-h-[50vh] overflow-y-auto border border-slate-200 rounded-lg px-4 py-2" data-trust="nda-body" @click="onNdaClick" v-html="ndaHtml"></div>
            <label class="flex items-start gap-2 text-sm">
              <input v-model="ndaAccepted" type="checkbox" class="mt-0.5" data-trust="nda-check" />
              <span>{{ ndaDoc.company ? $t('trust.public.nda_accept_label', { company: ndaDoc.company }) : $t('trust.public.nda_accept_label_plain') }}</span>
            </label>
            <p v-if="ndaError" class="text-sm text-red-700" role="alert" data-trust="nda-error">{{ ndaError }}</p>
          </template>
          <div class="flex items-center gap-2">
            <button v-if="ndaDoc && !ndaLoading" type="button" class="px-4 py-2 rounded-lg text-sm font-medium text-white hover:opacity-90 trust-fill disabled:opacity-60" data-trust="nda-accept" :disabled="!ndaAccepted || ndaSending" @click="acceptNDA">
              {{ ndaSending ? $t('trust.public.nda_accepting') : $t('trust.public.nda_accept') }}
            </button>
            <button type="button" class="px-3 py-2 rounded-lg text-sm text-slate-700 hover:bg-slate-100" data-trust="nda-cancel" @click="closeNDA">{{ $t('trust.public.nda_cancel') }}</button>
          </div>
        </div>
      </div>

      <footer class="border-t border-slate-200 bg-white">
        <div class="max-w-4xl mx-auto px-4 py-4 flex items-center justify-between gap-3 text-xs text-slate-500">
          <LocalePicker :persist="false" compact light />
          <a v-if="ov.show_powered_by" href="https://isms.sh" target="_blank" rel="noopener" class="hover:text-slate-700" data-trust="powered-by">{{ $t('trust.public.powered_by') }}</a>
        </div>
      </footer>
    </template>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch, watchEffect } from 'vue'
import DOMPurify from 'dompurify'
import api, { getApiToken } from '../api.js'
import { i18n, t } from '../i18n.js'
import { applyConfigLocales } from '../composables/useLocale.js'
import { applyBrandColor } from '../utils/brandColor.js'
import { compareTrustItems } from '../utils/trustOrder.js'
import { enumLabel } from '../composables/useEnumLabel.js'
import { saveBlob } from '../utils/saveBlob.js'
import { formatDayUTC, hostOf, safeHref } from '../utils/trustCertifications.js'
import LocalePicker from './LocalePicker.vue'

const props = defineProps({
  slug: { type: String, default: '' },
  // Where this page lives (/acme/trust, or /trust on a tenant host).
  basePath: { type: String, required: true },
  // The public id of the document page to show, when on one.
  docId: { type: String, default: '' },
  // Where the data comes from: {overview(), items(), document(id)}. The default
  // is the anonymous trust API; the manager's preview passes authenticated
  // loaders for the very same answers, plus certificationFile(itemId), which
  // returns { blob, filename } through the session.
  source: { type: Object, default: null },
})
// Navigation is the parent's job: the component renders real hrefs and hands a
// plain click up, so it needs no router to be mounted.
const emit = defineEmits(['navigate'])

const state = ref('loading') // loading | ready | unavailable | error
const ov = ref({})
const items = ref([])
const logoFailed = ref(false)
const filter = ref('all')
const hasSession = ref(false)

// Visitor sign-in (#524). Only the anonymous page has it: the manager's preview
// (a data source is given) is the anonymous view and never asks who is signed in.
const visitorFeatures = computed(() => !props.source)
const visitor = ref(null)
// Where the visitor stands with the organization's NDA (#525): {status: none |
// required | accepted | skipped | unavailable, ...}; null while signed out.
const nda = ref(null)
// Whether the anonymous sign-in affordances are shown. In the manager's preview
// they are shown too, because that is what visitors see, but inert: disabled, no
// form, and no visitor request (signInAvailable below is what makes requests).
const signInShown = computed(() => !!ov.value.visitor_sign_in)
const signInAvailable = computed(() => visitorFeatures.value && signInShown.value)
const formOpen = ref(false)
const email = ref('')
const sendState = ref('idle') // idle | sending | sent | error
const sendError = ref('')
const signingOut = ref(false)
const sendLabel = computed(() => t(sendState.value === 'sending' ? 'trust.public.sign_in_sending' : 'trust.public.sign_in_submit'))
// Only an http(s) link is rendered from the organization's setting.
const privacyUrl = computed(() => (/^https?:\/\//i.test(ov.value.privacy_notice_url || '') ? ov.value.privacy_notice_url : ''))

// The same rule as the server's (documents and certifications): public for
// everyone, gated for a signed-in visitor who has met the NDA step (no NDA set,
// accepted, or on the skip list). An NDA that is set but unavailable locks gated
// items for every visitor.
const ndaRequired = computed(() => !!ov.value.nda_required)
const ndaStatus = computed(() => (visitor.value ? (nda.value && nda.value.status) || 'none' : ''))
const mayRead = (d) => d.access_level === 'public' || (d.access_level === 'gated' && !!visitor.value && ['none', 'accepted', 'skipped'].includes(ndaStatus.value))
const needsSignIn = (d) => d.access_level === 'gated' && !visitor.value && signInShown.value
const needsNDA = (d) => d.access_level === 'gated' && visitorFeatures.value && ndaStatus.value === 'required'
const ndaLocked = (d) => d.access_level === 'gated' && visitorFeatures.value && ndaStatus.value === 'unavailable'
const ndaUnavailableNote = computed(() => visitorFeatures.value && ndaStatus.value === 'unavailable' && items.value.some((i) => i.access_level === 'gated'))
const signInLabel = computed(() => t(ndaRequired.value ? 'trust.public.action_sign_in_nda' : 'trust.public.action_sign_in'))

// The click-through NDA panel.
const ndaOpen = ref(false)
const ndaDoc = ref(null)
const ndaLoading = ref(false)
const ndaAccepted = ref(false)
const ndaSending = ref(false)
const ndaError = ref('')
const ndaChanged = ref(false)
let ndaAutoOpened = false
const ndaHtml = computed(() => (ndaDoc.value ? sanitize(ndaDoc.value.html) : ''))

async function openNDA() {
  if (!visitorFeatures.value) return
  ndaOpen.value = true
  ndaAccepted.value = false
  ndaError.value = ''
  ndaChanged.value = false
  ndaDoc.value = null
  ndaLoading.value = true
  try {
    ndaDoc.value = await api.trustNDA(props.slug)
  } catch (err) {
    // A 404 means the session ended, or the NDA is gone: ask again and let the
    // page show where the visitor stands now.
    if (err && err.status === 404) {
      await loadVisitor()
      await reloadItems()
      if (ndaStatus.value !== 'required') ndaOpen.value = false
    }
  } finally {
    ndaLoading.value = false
  }
}

function closeNDA() {
  ndaOpen.value = false
}

async function acceptNDA() {
  if (!ndaAccepted.value || ndaSending.value || !ndaDoc.value) return
  ndaSending.value = true
  ndaError.value = ''
  try {
    const r = await api.trustNDAAccept(props.slug, ndaDoc.value.content_hash)
    visitor.value = r.visitor || null
    nda.value = r.nda || null
    ndaOpen.value = false
    await reloadItems()
    if (props.docId && state.value === 'ready') await loadDocument()
  } catch (err) {
    if (err && err.status === 409 && err.body && err.body.nda) {
      // A newer version was approved while the text was open: show it and ask again.
      ndaDoc.value = err.body.nda
      ndaAccepted.value = false
      ndaChanged.value = true
    } else if (err && err.status === 404) {
      await loadVisitor()
      await reloadItems()
      if (ndaStatus.value !== 'required') ndaOpen.value = false
    } else {
      ndaError.value = t('trust.public.nda_error')
    }
  } finally {
    ndaSending.value = false
  }
}

function openSignIn() {
  if (!visitorFeatures.value) return
  formOpen.value = true
  sendState.value = 'idle'
  sendError.value = ''
}

async function sendLink() {
  if (sendState.value === 'sending') return
  sendState.value = 'sending'
  sendError.value = ''
  try {
    await api.trustRequestLink(props.slug, email.value.trim())
    sendState.value = 'sent'
  } catch (err) {
    sendState.value = 'error'
    if (err && err.code === 'trust_email_domain_blocked') sendError.value = t('common.error.trust_email_domain_blocked', { value: (err.params && err.params.value) || '' })
    else if (err && err.status === 429) sendError.value = t('trust.public.sign_in_rate_limited')
    else if (err && (err.status === 503 || err.code === 'trust_sign_in_unavailable')) sendError.value = t('trust.public.sign_in_unavailable')
    else if (err && err.status === 400) sendError.value = t('trust.public.sign_in_invalid_email')
    else sendError.value = t('trust.public.sign_in_error')
  }
}

async function loadVisitor() {
  visitor.value = null
  nda.value = null
  if (!signInAvailable.value) return
  try {
    const r = await api.trustVisitor(props.slug)
    visitor.value = r.visitor || null
    nda.value = visitor.value ? r.nda || null : null
  } catch {
    visitor.value = null
    nda.value = null
  }
}

async function signOut() {
  signingOut.value = true
  ndaOpen.value = false
  try {
    await api.trustSignOut(props.slug)
  } catch { /* the state is read again below either way */ }
  await loadVisitor()
  await reloadItems()
  signingOut.value = false
  if (state.value === 'ready') await loadDocument()
}

// The list carries a gated certification's file and link only for a signed-in
// visitor: read it again whenever the session ends, so they go with it.
async function reloadItems() {
  try {
    items.value = await loaders.value.items()
  } catch { /* the server no longer serves them either way */ }
}

// Asks the server whether the visitor shown is still signed in. When not, the
// page drops to the signed-out view (header and list). A failed check changes
// nothing.
async function visitorStillSignedIn(c) {
  let r
  try {
    r = await api.trustVisitor(props.slug)
  } catch {
    return true
  }
  if (!r.visitor) {
    visitor.value = null
    nda.value = null
    await reloadItems()
    return false
  }
  visitor.value = r.visitor
  nda.value = r.nda || null
  if (mayRead(c)) return true
  // Still signed in but no longer allowed: a new NDA version was approved while
  // the page was open. Show the NDA panel, not the signed-out view.
  await reloadItems()
  if (ndaStatus.value === 'required') await openNDA()
  return false
}

// A gated file is served only while the visitor's session lasts, and the session
// can end while the page is open: a manager ends it or blocks the visitor, a
// sign-out in another tab, expiry. A plain click on a gated Download therefore
// checks first, and when nobody is signed in any more the row turns back into
// "Sign in to view" instead of the download failing. Public files, and modified
// clicks (new tab, save link), are left to the browser.
async function downloadGated(e, c) {
  if (c.access_level === 'public' || e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
  e.preventDefault()
  if (!(await visitorStillSignedIn(c))) return
  const a = document.createElement('a')
  a.href = certFileHref(c)
  a.download = ''
  document.body.appendChild(a)
  a.click()
  a.remove()
}

const docState = ref('loading') // loading | ready | locked | missing | error
const doc = ref({})
const docHtml = ref('')

const loaders = computed(() => props.source || {
  overview: () => api.trustOverview(props.slug),
  items: () => api.trustItems(props.slug),
  document: (id) => api.trustDocument(props.slug, id),
})

const logoSrc = computed(() => `/branding/logo?org=${encodeURIComponent(props.slug)}`)

// Visitors never see anything but a trust API answer; the server's HTML is
// still sanitised here, because this is the last stop before v-html.
function sanitize(html) {
  const hook = (node) => {
    if (node.tagName === 'A' && node.getAttribute('href') && /^https?:/i.test(node.getAttribute('href'))) {
      node.setAttribute('target', '_blank')
      node.setAttribute('rel', 'noopener noreferrer')
    }
  }
  DOMPurify.addHook('afterSanitizeAttributes', hook)
  try {
    return DOMPurify.sanitize(html || '', { ADD_ATTR: ['data-trust-document'] })
  } finally {
    DOMPurify.removeHook('afterSanitizeAttributes', hook)
  }
}

const overviewHtml = computed(() => (ov.value.overview ? sanitize(ov.value.overview.html) : ''))
const hasOverview = computed(() => !!ov.value.overview && overviewHtml.value.trim() !== '')

const documents = computed(() =>
  items.value
    .filter((i) => i.kind === 'document')
    .sort(compareTrustItems),
)
const certifications = computed(() =>
  items.value
    .filter((i) => i.kind === 'certification')
    .sort(compareTrustItems),
)
const subprocessors = computed(() =>
  items.value
    .filter((i) => i.kind === 'subprocessor')
    .sort(compareTrustItems),
)
const certStateLabel = (s) => t(s === 'expiring' ? 'trust.public.certifications.state_expiring' : 'trust.public.certifications.state_current')
const certStateClass = (s) => (s === 'expiring' ? 'bg-amber-100 text-amber-800' : 'bg-emerald-100 text-emerald-800')
const certTypeLabel = (v) => enumLabel('external_link_type', v)
const fmtDay = (d) => formatDayUTC(d, i18n.global.locale.value)
// The anonymous file route; a preview passes source.certificationFile instead
// and downloads with the session.
const certFileHref = (c) => `/api/v1/trust/${encodeURIComponent(props.slug)}/certifications/${encodeURIComponent(c.id)}/file`
async function downloadCert(c) {
  try {
    const { blob, filename } = await props.source.certificationFile(c.id)
    saveBlob(blob, filename || c.file_name)
  } catch {
    // The preview shows what a visitor gets; a failure here is the visitor's 404.
  }
}
const filterLevel = { public: 'public', gated: 'gated', restricted: 'restricted' }
const visibleDocuments = computed(() =>
  filter.value === 'all' ? documents.value : documents.value.filter((d) => d.access_level === filterLevel[filter.value]),
)
const filters = computed(() => {
  const have = (lvl) => documents.value.some((d) => d.access_level === lvl)
  return [
    { key: 'all', label: t('trust.public.filter_all') },
    have('public') && { key: 'public', label: t('trust.public.filter_public') },
    have('gated') && { key: 'gated', label: t(ndaRequired.value ? 'trust.public.filter_gated_nda' : 'trust.public.filter_gated') },
    have('restricted') && { key: 'restricted', label: t('trust.public.filter_restricted') },
  ].filter(Boolean)
})

function accessLabel(level) {
  return t(level === 'gated' && ndaRequired.value ? 'trust.public.access_gated_nda' : `trust.public.access_${level}`)
}
const isEmail = (v) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v || '')
const shortHash = (h) => (h || '').slice(0, 10)
function fmtDate(sec) {
  try {
    return new Intl.DateTimeFormat(i18n.global.locale.value, { dateStyle: 'medium' }).format(new Date(sec * 1000))
  } catch {
    return new Date(sec * 1000).toISOString().slice(0, 10)
  }
}

// Real links are handled in-app only for a plain left click; a modified or
// non-primary click (new tab, new window, download) is left to the browser.
function go(e, path) {
  if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
  e.preventDefault()
  emit('navigate', path)
}

// A link inside server HTML to another published document has no href (the
// server cannot know the hosting mode): route it from here.
function onContentClick(e) {
  const a = e.target && e.target.closest ? e.target.closest('a[data-trust-document]') : null
  if (!a) return false
  e.preventDefault()
  emit('navigate', `${props.basePath}/documents/${a.getAttribute('data-trust-document')}`)
  return true
}
// A link in the NDA text that leaves for another document closes the panel, so it
// does not stay open over the page the visitor went to.
function onNdaClick(e) {
  if (onContentClick(e)) ndaOpen.value = false
}

const lockedItem = ref({})

async function loadDocument() {
  if (!props.docId) return
  const id = props.docId.toLowerCase()
  const it = documents.value.find((d) => d.id === id)
  if (!it) {
    docState.value = 'missing'
    return
  }
  if (!mayRead(it)) {
    lockedItem.value = it
    docState.value = 'locked'
    return
  }
  docState.value = 'loading'
  try {
    const d = await loaders.value.document(id)
    doc.value = d
    docHtml.value = sanitize(d.html)
    docState.value = 'ready'
  } catch (err) {
    // A 404 while we think a visitor is signed in may mean the session ended
    // (revoked, blocked, expired): ask again, and if they are signed out show the
    // locked state with its sign-in prompt, not "missing".
    // The same 404 also means a new NDA version was approved while the page was
    // open: then the visitor is still signed in and needs to accept it again.
    if (err && err.status === 404 && visitor.value && visitorFeatures.value) {
      await loadVisitor()
      if (!mayRead(it)) {
        lockedItem.value = it
        docState.value = 'locked'
        if (ndaStatus.value === 'required') await openNDA()
        return
      }
    }
    docState.value = err && err.status === 404 ? 'missing' : 'error'
  }
}

async function load() {
  if (!props.slug) {
    state.value = 'unavailable'
    return
  }
  state.value = 'loading'
  hasSession.value = !!getApiToken()
  try {
    const [o, list, cfg] = await Promise.all([
      loaders.value.overview(),
      loaders.value.items(),
      api.publicConfig().catch(() => null),
    ])
    ov.value = o
    items.value = list
    // Brand first, so nothing flashes unbranded; then the language.
    applyBrandColor(o.branding_color)
    if (cfg && cfg.locales) await applyConfigLocales({ locales: cfg.locales, default_locale: o.default_locale })
    await loadVisitor()
    state.value = 'ready'
    if (typeof location !== 'undefined' && location.hash === '#sign-in' && signInAvailable.value && !visitor.value) openSignIn()
    await loadDocument()
    // Someone who just signed in and has not accepted the NDA is asked once.
    if (visitor.value && ndaStatus.value === 'required' && !ndaAutoOpened && !ndaOpen.value) {
      ndaAutoOpened = true
      await openNDA()
    }
  } catch (err) {
    state.value = err && err.status === 404 ? 'unavailable' : 'error'
  }
}

watch(() => props.docId, () => { if (state.value === 'ready') loadDocument() })
watch(() => props.slug, load)

// ---- global side effects, undone on unmount -------------------------------
// The page borrows the document title, the favicon, the page background and the
// brand colour variables from the app. Going back to the app must leave all of
// them as they were.
const BRAND_VARS = ['--brand-color', '--brand-text-color', '--brand-fill-color']
let saved = null
let createdFavicon = null

onMounted(() => {
  const root = document.documentElement.style
  const link = document.querySelector('link[rel~="icon"]')
  saved = {
    title: document.title,
    bodyBg: document.body.style.background,
    rootBg: root.background,
    brand: BRAND_VARS.map((n) => [n, root.getPropertyValue(n)]),
    link,
    href: link ? link.getAttribute('href') : null,
  }
  document.body.style.background = '#f8fafc'
  root.background = '#f8fafc'
  load()
})

watchEffect(() => {
  if (state.value !== 'ready' || !saved) return
  const name = ov.value.branding_name || ''
  document.title = ov.value.title || t('trust.public.page_title', { name })
  const href = `/branding/favicon.ico?org=${encodeURIComponent(props.slug)}`
  if (saved.link) saved.link.setAttribute('href', href)
  else if (!createdFavicon) {
    createdFavicon = document.createElement('link')
    createdFavicon.setAttribute('rel', 'icon')
    createdFavicon.setAttribute('href', href)
    document.head.appendChild(createdFavicon)
  }
})

onBeforeUnmount(() => {
  if (!saved) return
  const root = document.documentElement.style
  document.title = saved.title
  document.body.style.background = saved.bodyBg
  root.background = saved.rootBg
  for (const [name, value] of saved.brand) {
    if (value) root.setProperty(name, value)
    else root.removeProperty(name)
  }
  if (saved.link && saved.href !== null) saved.link.setAttribute('href', saved.href)
  if (createdFavicon) createdFavicon.remove()
  createdFavicon = null
})
</script>

<style scoped>
/* Brand colour on a LIGHT page. --brand-text-color is tuned to be read on the
   app's dark surface and would wash out here; --brand-fill-color is darkened
   until white text reaches 4.5:1, which also makes it readable text on white. */
.trust-brand-text {
  color: var(--brand-fill-color, #3472d8);
}
.trust-fill {
  background-color: var(--brand-fill-color, #3472d8);
}
</style>

<style>
/* Server-rendered markdown on the light trust page. The app's .doc-prose is dark
   (code on slate-800), so the page carries its own. */
.trust-prose { line-height: 1.65; color: #0f172a; overflow-wrap: anywhere; }
.trust-prose > :first-child { margin-top: 0; }
.trust-prose h1 { font-size: 1.6rem; font-weight: 700; margin: 1.6em 0 .6em; }
.trust-prose h2 { font-size: 1.3rem; font-weight: 600; margin: 1.5em 0 .5em; }
.trust-prose h3 { font-size: 1.1rem; font-weight: 600; margin: 1.3em 0 .4em; }
.trust-prose h4, .trust-prose h5, .trust-prose h6 { font-weight: 600; margin: 1.2em 0 .3em; }
.trust-prose p { margin: .8em 0; }
.trust-prose ul { list-style: disc; padding-left: 1.5em; margin: .8em 0; }
.trust-prose ol { list-style: decimal; padding-left: 1.5em; margin: .8em 0; }
.trust-prose li { margin: .25em 0; }
.trust-prose li > input[type='checkbox'] { margin-right: .5em; }
.trust-prose ul:has(> li > input[type='checkbox']) { list-style: none; padding-left: .25em; }
.trust-prose a { color: var(--brand-fill-color, #3472d8); text-decoration: underline; cursor: pointer; }
.trust-prose blockquote { border-left: 3px solid #cbd5e1; padding-left: 1em; color: #475569; margin: 1em 0; }
.trust-prose code { background: #f1f5f9; border: 1px solid #e2e8f0; border-radius: .25rem; padding: .05em .35em; font-size: .9em; }
.trust-prose pre { background: #f1f5f9; border: 1px solid #e2e8f0; border-radius: .5rem; padding: .75em 1em; overflow-x: auto; margin: 1em 0; }
.trust-prose pre code { background: none; border: 0; padding: 0; }
.trust-prose hr { border: 0; border-top: 1px solid #e2e8f0; margin: 1.5em 0; }
.trust-prose img { max-width: 100%; height: auto; }
.trust-prose table { display: block; max-width: 100%; overflow-x: auto; border-collapse: collapse; margin: 1em 0; }
.trust-prose th, .trust-prose td { border: 1px solid #cbd5e1; padding: .4em .7em; text-align: left; vertical-align: top; }
.trust-prose th { background: #f1f5f9; font-weight: 600; }
</style>
