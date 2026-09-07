<template>
  <div class="min-h-screen bg-[#0a0e1a] text-white overflow-x-hidden">
    <!-- Grid background -->
    <div class="fixed inset-0 pointer-events-none" style="background-image:linear-gradient(color-mix(in srgb, var(--brand-color) 4%, transparent) 1px, transparent 1px),linear-gradient(90deg, color-mix(in srgb, var(--brand-color) 4%, transparent) 1px, transparent 1px);background-size:60px 60px;"></div>

    <!-- Nav -->
    <nav class="relative z-10 flex items-center justify-between px-8 py-5 max-w-7xl mx-auto">
      <router-link to="/" class="flex items-center gap-3 hover:opacity-80 transition-opacity">
        <img v-if="logoUrl && !logoError" :src="logoUrl" @error="logoError = true" alt="" class="h-9 w-auto" />
        <div v-else class="w-9 h-9 rounded-xl flex items-center justify-center" style="background: linear-gradient(to bottom right, var(--brand-color), color-mix(in srgb, var(--brand-color) 70%, black))">
          <svg class="w-5 h-5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z" />
          </svg>
        </div>
        <span class="text-xl font-bold tracking-tight">{{ brandName }}</span>
      </router-link>
      <div class="flex items-center gap-5">
        <a href="/docs" class="text-sm text-slate-400 hover:text-white transition-colors">{{ $t('landing.nav.docs') }}</a>
        <a href="https://github.com/unidoc/isms" target="_blank" rel="noopener" class="text-sm text-slate-400 hover:text-white transition-colors flex items-center gap-1.5">
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor"><path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/></svg>
          <!-- i18n-ignore: brand name -->
          GitHub
        </a>
        <router-link to="/login" class="px-5 py-2 text-sm font-medium bg-white/10 hover:bg-white/15 border border-white/10 rounded-lg transition-all hover:border-white/20">{{ $t('landing.nav.sign_in') }}</router-link>
      </div>
    </nav>

    <!-- Hero -->
    <section class="relative z-10 max-w-5xl mx-auto px-8 pt-20 pb-12 text-center">
      <div class="inline-flex items-center gap-2 px-4 py-1.5 mb-8 text-xs font-medium text-slate-400 bg-slate-800/50 border border-slate-700/50 rounded-full">
        <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
        {{ $t('landing.hero.badge') }}
      </div>
      <h1 class="text-5xl sm:text-7xl font-extrabold tracking-tight leading-[1.1] mb-6">
        <span class="bg-gradient-to-r from-white via-white to-slate-400 bg-clip-text text-transparent">{{ $t('landing.hero.title_line1') }}</span><br>
        <span class="hero-accent-gradient bg-clip-text text-transparent">{{ $t('landing.hero.title_line2') }}</span>
      </h1>
      <p class="text-lg sm:text-xl text-slate-400 max-w-2xl mx-auto mb-10 leading-relaxed">
        {{ $t('landing.hero.subtitle') }}
      </p>
      <div class="flex items-center justify-center gap-4 flex-wrap">
        <router-link v-if="signupEnabled" to="/signup"
          class="brand-cta-btn inline-flex items-center gap-2 px-7 py-3 text-white font-semibold rounded-xl transition-all hover:-translate-y-0.5">
          {{ $t('landing.hero.cta_cloud') }}
          <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5"><path stroke-linecap="round" stroke-linejoin="round" d="M13.5 4.5L21 12m0 0l-7.5 7.5M21 12H3"/></svg>
        </router-link>
        <a href="https://github.com/unidoc/isms" target="_blank" rel="noopener"
          class="inline-flex items-center gap-2 px-6 py-3 text-sm font-medium text-slate-300 bg-slate-800/50 border border-slate-700 rounded-xl hover:bg-slate-800 hover:border-slate-600 transition-all">
          {{ $t('landing.hero.cta_selfhost') }}
        </a>
        <a href="/docs"
          class="inline-flex items-center gap-2 px-6 py-3 text-sm font-medium text-slate-400 hover:text-slate-200 transition-colors">
          {{ $t('landing.hero.cta_docs') }}
        </a>
      </div>
    </section>

    <!-- Why — lead with the conviction, not the feature list (#45) -->
    <section class="relative z-10 max-w-3xl mx-auto px-8 pt-8 pb-16">
      <h2 class="text-3xl font-bold text-center mb-8">{{ $t('landing.why.heading') }}</h2>
      <div class="space-y-5 text-slate-400 leading-relaxed text-lg">
        <p>{{ $t('landing.why.p1') }}</p>
        <p class="text-slate-200">{{ $t('landing.why.p2') }}</p>
        <p>{{ $t('landing.why.p3') }}</p>
      </div>
    </section>

    <!-- Thesis: 3 pillars -->
    <section class="relative z-10 max-w-5xl mx-auto px-8 py-16">
      <div class="grid md:grid-cols-3 gap-8">
        <div class="text-center">
          <div class="w-12 h-12 rounded-xl bg-emerald-500/10 flex items-center justify-center mx-auto mb-4">
            <svg class="w-6 h-6 text-emerald-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z" /></svg>
          </div>
          <h3 class="text-lg font-semibold mb-2">{{ $t('landing.pillars.documents_title') }}</h3>
          <p class="text-sm text-slate-400 leading-relaxed">{{ $t('landing.pillars.documents_desc') }}</p>
        </div>
        <div class="text-center">
          <div class="w-12 h-12 rounded-xl bg-blue-500/10 flex items-center justify-center mx-auto mb-4">
            <svg class="w-6 h-6 text-blue-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75L11.25 15 15 9.75M21 12c0 1.268-.63 2.39-1.593 3.068a3.745 3.745 0 01-1.043 3.296 3.745 3.745 0 01-3.296 1.043A3.745 3.745 0 0112 21c-1.268 0-2.39-.63-3.068-1.593a3.746 3.746 0 01-3.296-1.043 3.745 3.745 0 01-1.043-3.296A3.745 3.745 0 013 12c0-1.268.63-2.39 1.593-3.068a3.745 3.745 0 011.043-3.296 3.746 3.746 0 013.296-1.043A3.746 3.746 0 0112 3c1.268 0 2.39.63 3.068 1.593a3.746 3.746 0 013.296 1.043 3.745 3.745 0 011.043 3.296A3.745 3.745 0 0121 12z" /></svg>
          </div>
          <h3 class="text-lg font-semibold mb-2">{{ $t('landing.pillars.reviews_title') }}</h3>
          <p class="text-sm text-slate-400 leading-relaxed">{{ $t('landing.pillars.reviews_desc') }}</p>
        </div>
        <div class="text-center">
          <div class="w-12 h-12 rounded-xl bg-amber-500/10 flex items-center justify-center mx-auto mb-4">
            <svg class="w-6 h-6 text-amber-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M6.75 3v2.25M17.25 3v2.25M3 18.75V7.5a2.25 2.25 0 012.25-2.25h13.5A2.25 2.25 0 0121 7.5v11.25m-18 0A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75m-18 0v-7.5A2.25 2.25 0 015.25 9h13.5A2.25 2.25 0 0121 11.25v7.5" /></svg>
          </div>
          <h3 class="text-lg font-semibold mb-2">{{ $t('landing.pillars.tasks_title') }}</h3>
          <p class="text-sm text-slate-400 leading-relaxed">{{ $t('landing.pillars.tasks_desc') }}</p>
        </div>
      </div>
    </section>

    <!-- How it works -->
    <section class="relative z-10 max-w-4xl mx-auto px-8 py-16">
      <h2 class="text-3xl font-bold text-center mb-4">{{ $t('landing.how.heading') }}</h2>
      <p class="text-slate-400 text-center mb-12 max-w-2xl mx-auto">{{ $t('landing.how.subtitle') }}</p>
      <div class="space-y-4">
        <div v-for="(step, i) in steps" :key="i" class="flex gap-4 items-start p-5 bg-slate-900/30 border border-slate-800/30 rounded-xl">
          <div class="w-8 h-8 rounded-full flex items-center justify-center flex-shrink-0 text-sm font-bold text-white" style="background-color: var(--brand-color)">{{ i + 1 }}</div>
          <div>
            <h4 class="font-semibold mb-1">{{ step.title }}</h4>
            <p class="text-sm text-slate-400">{{ step.desc }}</p>
          </div>
        </div>
      </div>
    </section>

    <!-- Features: why isms.sh -->
    <section class="relative z-10 max-w-6xl mx-auto px-8 py-16">
      <h2 class="text-3xl font-bold text-center mb-4">{{ $t('landing.features.heading') }}</h2>
      <p class="text-slate-400 text-center mb-12 max-w-2xl mx-auto">{{ $t('landing.features.subtitle') }}</p>
      <div class="grid md:grid-cols-2 gap-6">
        <div v-for="f in features" :key="f.title" class="p-6 bg-slate-900/50 border border-slate-800/50 rounded-2xl backdrop-blur-sm hover:border-slate-700/50 transition-colors">
          <h3 class="text-base font-semibold mb-2">{{ f.title }}</h3>
          <p class="text-sm text-slate-400 leading-relaxed">{{ f.desc }}</p>
        </div>
      </div>
    </section>

    <!-- Frameworks -->
    <section class="relative z-10 max-w-4xl mx-auto px-8 py-16 text-center">
      <h2 class="text-3xl font-bold mb-4">{{ $t('landing.frameworks.heading') }}</h2>
      <p class="text-slate-400 mb-10 max-w-2xl mx-auto">{{ $t('landing.frameworks.subtitle') }}</p>
      <div class="flex flex-wrap justify-center gap-3">
        <span v-for="fw in frameworks" :key="fw" class="px-4 py-2 bg-slate-800/50 border border-slate-700/50 rounded-lg text-sm text-slate-300">{{ fw }}</span>
      </div>
    </section>

    <!-- Deploy -->
    <section class="relative z-10 max-w-4xl mx-auto px-8 py-16">
      <div class="grid md:grid-cols-2 gap-6">
        <div class="p-6 bg-slate-900/50 border border-slate-800/50 rounded-2xl">
          <h3 class="text-lg font-semibold mb-2">{{ $t('landing.deploy.cloud_title') }}</h3>
          <p class="text-sm text-slate-400 leading-relaxed mb-4">{{ $t('landing.deploy.cloud_desc') }}</p>
          <router-link v-if="signupEnabled" to="/signup" class="text-sm font-medium hover:underline" style="color: var(--brand-color)">{{ $t('landing.deploy.cloud_cta') }} &rarr;</router-link> <!-- i18n-ignore: typographic arrow -->
          <router-link v-else to="/login" class="text-sm font-medium hover:underline" style="color: var(--brand-color)">{{ $t('landing.deploy.cloud_cta_signin') }} &rarr;</router-link> <!-- i18n-ignore: typographic arrow -->
        </div>
        <div class="p-6 bg-slate-900/50 border border-slate-800/50 rounded-2xl">
          <h3 class="text-lg font-semibold mb-2">{{ $t('landing.deploy.selfhost_title') }}</h3>
          <p class="text-sm text-slate-400 leading-relaxed mb-4">{{ $t('landing.deploy.selfhost_desc') }}</p>
          <a href="https://github.com/unidoc/isms" target="_blank" rel="noopener" class="text-sm font-medium text-slate-400 hover:text-white hover:underline">{{ $t('landing.deploy.selfhost_cta') }} &rarr;</a> <!-- i18n-ignore: typographic arrow -->
        </div>
      </div>
    </section>

    <!-- CTA -->
    <section class="relative z-10 max-w-3xl mx-auto px-8 py-20 text-center">
      <h2 class="text-4xl font-bold mb-4">{{ $t('landing.cta.heading') }}</h2>
      <p class="text-slate-400 mb-8">{{ $t('landing.cta.subtitle') }}</p>
      <div class="flex items-center justify-center gap-4 flex-wrap">
        <router-link v-if="signupEnabled" to="/signup"
          class="brand-cta-btn inline-flex items-center gap-2 px-7 py-3 text-white font-semibold rounded-xl transition-all hover:-translate-y-0.5">
          {{ $t('landing.hero.cta_cloud') }}
        </router-link>
        <a href="https://github.com/unidoc/isms" target="_blank" rel="noopener"
          class="inline-flex items-center gap-2 px-6 py-3 text-sm font-medium text-slate-300 bg-slate-800/50 border border-slate-700 rounded-xl hover:bg-slate-800 hover:border-slate-600 transition-all">
          {{ $t('landing.hero.cta_selfhost') }}
        </a>
      </div>
    </section>

    <!-- Footer -->
    <footer class="relative z-10 border-t border-slate-800/50 px-8 py-8 mt-8">
      <div class="max-w-6xl mx-auto flex items-center justify-between gap-4 text-sm text-slate-500">
        <span>{{ brandFooter }}</span>
        <div class="flex items-center gap-4">
          <!-- Pre-login, so the choice is local only: there is no account to save
               it against yet. -->
          <LocalePicker :persist="false" compact class="shrink-0" />
          <i18n-t v-if="showPoweredBy" keypath="landing.footer.powered_by" tag="span" scope="global">
            <template #link>
              <!-- i18n-ignore: product domain -->
              <a href="https://isms.sh" target="_blank" rel="noopener" class="text-slate-400 hover:text-white transition-colors">isms.sh</a>
            </template>
          </i18n-t>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup>
import { computed, ref, onMounted } from 'vue'
import api from '../api'
import { applyConfigLocales } from '../composables/useLocale'
import LocalePicker from '../components/LocalePicker.vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

// Neutral seed so a self-hosted deployment doesn't flash "isms.sh" in the
// top-left while /api/v1/config resolves the branded name — same fallback
// App.vue's sidebar already uses for orgName.
const brandName = ref(t('common.brand.product_name'))
const brandFooter = ref('')
const logoUrl = ref(null)
const logoError = ref(false)
const showPoweredBy = ref(true)
const signupEnabled = ref(false)

// computed, not a plain array: the footer carries a LocalePicker, so the locale
// can change while this page is mounted. A literal array would be built once
// against the boot locale and never re-render.
const features = computed(() => [
  { title: t('landing.features.git_title'), desc: t('landing.features.git_desc') },
  { title: t('landing.features.review_title'), desc: t('landing.features.review_desc') },
  { title: t('landing.features.registers_title'), desc: t('landing.features.registers_desc') },
  { title: t('landing.features.ai_title'), desc: t('landing.features.ai_desc') },
  { title: t('landing.features.cadence_title'), desc: t('landing.features.cadence_desc') },
  { title: t('landing.features.audit_title'), desc: t('landing.features.audit_desc') },
])

const steps = computed(() => [
  { title: t('landing.how.step1_title'), desc: t('landing.how.step1_desc') },
  { title: t('landing.how.step2_title'), desc: t('landing.how.step2_desc') },
  { title: t('landing.how.step3_title'), desc: t('landing.how.step3_desc') },
  { title: t('landing.how.step4_title'), desc: t('landing.how.step4_desc') },
  { title: t('landing.how.step5_title'), desc: t('landing.how.step5_desc') },
])

const frameworks = [
  'ISO 27001', 'ISO 9001', 'ISO 14001', 'ISO 42001', 'SOC 2',
  'NIS2', 'HIPAA', 'PCI DSS', 'NIST CSF', 'NIST 800-53', 'DORA',
]

onMounted(async () => {
  try {
    const cfg = await api.getConfig()
    await applyConfigLocales(cfg)
    if (cfg.branding?.branding_name) {
      brandName.value = cfg.branding.branding_name
    } else if (cfg.organization_name) {
      brandName.value = cfg.organization_name
    }
    if (cfg.branding?.branding_footer) {
      brandFooter.value = cfg.branding.branding_footer
    }
    if (cfg.branding?.branding_color) document.documentElement.style.setProperty('--brand-color', cfg.branding.branding_color)
    logoUrl.value = cfg.branding?.branding_logo || ''
    showPoweredBy.value = cfg.show_powered_by !== false
    signupEnabled.value = cfg.signup_enabled === true
  } catch {
    // Config endpoint may not be available pre-auth
  }
})
</script>

<style scoped>
.hero-accent-gradient {
  background-image: linear-gradient(to right, var(--brand-color), color-mix(in srgb, var(--brand-color) 60%, #c084fc));
}

.brand-cta-btn {
  background: linear-gradient(to right, var(--brand-color), color-mix(in srgb, var(--brand-color) 85%, white));
  box-shadow: 0 10px 15px -3px color-mix(in srgb, var(--brand-color) 25%, transparent);
}
.brand-cta-btn:hover {
  filter: brightness(1.12);
  box-shadow: 0 10px 15px -3px color-mix(in srgb, var(--brand-color) 40%, transparent);
}
</style>
