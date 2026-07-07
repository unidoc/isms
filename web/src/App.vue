<template>
  <div class="min-h-screen bg-slate-950 text-slate-200">
    <!-- Public pages (landing, login, organizations) — no sidebar -->
    <div v-if="isPublicRoute || route.path === '/organizations'">
      <router-view />
    </div>

    <!-- App layout with sidebar + header -->
    <div v-else class="flex min-h-screen">
      <!-- Sidebar (nav only) -->
      <!-- Mobile overlay -->
      <div v-if="isMobile && sidebarMobileOpen" @click="sidebarMobileOpen = false" class="fixed inset-0 bg-black/50 z-20" />
      <aside class="bg-slate-900 border-r border-slate-800 flex flex-col fixed inset-y-0 left-0 z-30 transition-all duration-200"
        :class="[
          sidebarCollapsed ? 'w-14' : 'w-60',
          isMobile && !sidebarMobileOpen ? '-translate-x-full' : 'translate-x-0',
        ]">

        <!-- Logo + org name -->
        <router-link :to="orgHomeLink" class="flex items-center border-b border-slate-800 hover:opacity-90 transition-opacity" :class="sidebarCollapsed ? 'px-3 py-4 justify-center' : 'px-4 py-4 gap-3'" :title="sidebarCollapsed ? (orgName || 'ISMS') : undefined">
          <img v-if="logoUrl && !logoError" :src="logoUrl" @error="logoError = true" alt="" class="flex-shrink-0 object-contain" :class="sidebarCollapsed ? 'h-8 w-8' : 'h-9 max-w-[8rem]'" />
          <div v-else class="flex-shrink-0 rounded-lg flex items-center justify-center" :class="sidebarCollapsed ? 'w-8 h-8' : 'w-9 h-9'" style="background: var(--brand-color, #3b82f6)">
            <svg class="w-4 h-4 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z" />
            </svg>
          </div>
          <div v-if="!sidebarCollapsed" class="min-w-0">
            <div class="text-sm font-bold truncate brand-org-name">{{ orgName || 'ISMS' }}</div>
            <div v-if="orgSlug" class="text-[10px] text-slate-500 truncate">{{ orgSlug }}</div>
          </div>
        </router-link>

        <!-- Nav items (no org switcher here — it's in the header) -->
        <div v-if="!hasOrg && !sidebarCollapsed" class="px-3 py-3 border-b border-slate-800">
          <button @click="showOrgMenu = !showOrgMenu" class="w-full flex items-center gap-2 px-2 py-2 rounded-lg text-left hover:bg-slate-800 transition-colors">
            <div class="w-7 h-7 rounded-lg bg-blue-600/20 text-blue-400 flex items-center justify-center text-xs font-bold flex-shrink-0">
              {{ (orgName || orgSlug || '?').charAt(0).toUpperCase() }}
            </div>
            <div class="flex-1 min-w-0">
              <div class="text-sm font-medium text-white truncate">{{ orgName || orgSlug || 'No org' }}</div>
              <div class="text-[10px] text-slate-500 truncate">{{ currentUserData?.organization_slug }}</div>
            </div>
            <svg class="w-3 h-3 text-slate-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 8.25l-7.5 7.5-7.5-7.5"/>
            </svg>
          </button>

          <!-- Org dropdown -->
          <div v-if="showOrgMenu" class="absolute left-3 right-3 top-auto mt-1 bg-slate-800 border border-slate-700 rounded-xl shadow-xl z-50 py-1">
            <!-- Current org header -->
            <div class="px-4 py-2.5 border-b border-slate-700">
              <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Current organization</div>
              <div class="text-sm font-medium text-white">{{ orgName }}</div>
              <div class="text-xs text-slate-500">{{ currentUserData?.organization_slug }}</div>
            </div>

            <!-- Org settings (admin/manager only) -->
            <div v-if="currentUserData?.role === 'admin'" class="py-1 border-b border-slate-700">
              <router-link :to="orgPath('/admin/settings')" @click="showOrgMenu = false"
                class="w-full flex items-center gap-2 px-4 py-2 text-sm text-slate-300 hover:text-white hover:bg-slate-700/50 transition-colors">
                <svg class="w-4 h-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
                  <path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                </svg>
                Organization settings
              </router-link>
            </div>

            <!-- Switch to other orgs -->
            <div v-if="otherOrgs.length > 0" class="py-1 border-b border-slate-700">
              <div class="px-4 py-1 text-[10px] text-slate-500 uppercase tracking-wider">Switch to</div>
              <button v-for="org in otherOrgs" :key="org.slug" @click="switchOrg(org)" class="w-full px-4 py-2 text-left text-sm text-slate-300 hover:text-white hover:bg-slate-700/50 transition-colors flex items-center justify-between">
                <span>{{ org.name }}</span>
                <span class="text-[10px] px-1.5 py-0.5 rounded-full" :class="org.role === 'admin' ? 'bg-purple-500/20 text-purple-300' : 'bg-slate-500/20 text-slate-400'">{{ org.role }}</span>
              </button>
            </div>

            <!-- Create new org -->
            <router-link to="/organizations" @click="showOrgMenu = false"
              class="flex items-center gap-2 px-4 py-2.5 text-sm text-slate-400 hover:text-white hover:bg-slate-700/50 transition-colors">
              <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15"/></svg>
              Create new organization
            </router-link>
          </div>
        </div>

        <!-- Click-away overlay for org menu -->
        <div v-if="showOrgMenu" @click="showOrgMenu = false" class="fixed inset-0 z-20" />

        <!-- Nav items -->
        <nav class="flex-1 overflow-y-auto py-3" :class="sidebarCollapsed ? 'px-1.5' : 'px-3'">
          <!-- Search -->
          <div class="space-y-0.5 mb-1">
            <button v-if="hasOrg" @click="globalSearchRef?.open()"
              class="w-full flex items-center gap-2.5 py-1.5 text-sm rounded-lg transition-colors text-slate-400 hover:text-white hover:bg-slate-800/50"
              :class="sidebarCollapsed ? 'px-2.5 justify-center' : 'px-2'"
              :title="sidebarCollapsed ? 'Search' : undefined">
              <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="m21 21-5.197-5.197m0 0A7.5 7.5 0 1 0 5.196 5.196a7.5 7.5 0 0 0 10.607 10.607Z" />
              </svg>
              <span v-if="!sidebarCollapsed" class="flex-1 text-left">Search</span>
              <kbd v-if="!sidebarCollapsed" class="text-[10px] font-mono text-slate-600">⌘K</kbd>
            </button>
          </div>

          <!-- Group: Core -->
          <div class="space-y-0.5">
            <router-link v-if="hasOrg" :to="orgPath('/overview')"
              class="flex items-center gap-2.5 py-1.5 text-sm rounded-lg transition-colors"
              :class="[route.path.startsWith(orgPath('/overview')) ? 'bg-slate-800 text-white font-medium' : 'text-slate-400 hover:text-white hover:bg-slate-800/50', sidebarCollapsed ? 'px-2.5 justify-center' : 'px-2']"
              :title="sidebarCollapsed ? 'Overview' : undefined">
              <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M3 13.125C3 12.504 3.504 12 4.125 12h2.25c.621 0 1.125.504 1.125 1.125v6.75C7.5 20.496 6.996 21 6.375 21h-2.25A1.125 1.125 0 013 19.875v-6.75zM9.75 8.625c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125v11.25c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V8.625zM16.5 4.125c0-.621.504-1.125 1.125-1.125h2.25C20.496 3 21 3.504 21 4.125v15.75c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V4.125z" />
              </svg>
              <span v-if="!sidebarCollapsed">Overview</span>
            </router-link>
            <router-link v-if="hasOrg" :to="orgPath('/documents')"
              class="flex items-center gap-2.5 py-1.5 text-sm rounded-lg transition-colors"
              :class="[route.path.startsWith(orgPath('/documents')) ? 'bg-slate-800 text-white font-medium' : 'text-slate-400 hover:text-white hover:bg-slate-800/50', sidebarCollapsed ? 'px-2.5 justify-center' : 'px-2']"
              :title="sidebarCollapsed ? 'Documents' : undefined">
              <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z" />
              </svg>
              <span v-if="!sidebarCollapsed">Documents</span>
            </router-link>
            <router-link v-if="hasOrg" :to="orgPath('/reviews')"
              class="relative flex items-center gap-2.5 py-1.5 text-sm rounded-lg transition-colors"
              :class="[route.path.startsWith(orgPath('/reviews')) ? 'bg-slate-800 text-white font-medium' : 'text-slate-400 hover:text-white hover:bg-slate-800/50', sidebarCollapsed ? 'px-2.5 justify-center' : 'px-2']"
              :title="sidebarCollapsed ? 'Reviews' : undefined">
              <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M7.5 21L3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5" />
              </svg>
              <template v-if="!sidebarCollapsed">
                <span class="flex-1">Reviews</span>
                <span v-if="openReviewCount > 0" class="min-w-[20px] h-5 px-1 bg-blue-500 text-white text-[10px] font-bold rounded-full flex items-center justify-center flex-shrink-0">
                  {{ openReviewCount > 99 ? '99+' : openReviewCount }}
                </span>
              </template>
              <span v-else-if="openReviewCount > 0" class="absolute -top-1 -right-1 w-4 h-4 bg-blue-500 text-white text-[8px] font-bold rounded-full flex items-center justify-center">
                {{ openReviewCount > 9 ? '9+' : openReviewCount }}
              </span>
            </router-link>
            <div v-if="hasOrg" class="relative flex items-center">
              <router-link :to="orgPath('/inbox')"
                class="relative flex items-center gap-2.5 py-1.5 text-sm rounded-lg transition-colors flex-1"
                :class="[route.path.startsWith(orgPath('/inbox')) ? 'bg-slate-800 text-white font-medium' : 'text-slate-400 hover:text-white hover:bg-slate-800/50', sidebarCollapsed ? 'px-2.5 justify-center' : 'px-2']"
                :title="sidebarCollapsed ? 'Inbox' : undefined">
                <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M2.25 13.5h3.86a2.25 2.25 0 012.012 1.244l.256.512a2.25 2.25 0 002.013 1.244h3.218a2.25 2.25 0 002.013-1.244l.256-.512a2.25 2.25 0 012.013-1.244h3.859M12 3v8.25m0 0l-3-3m3 3l3-3" />
                </svg>
                <span v-if="!sidebarCollapsed" class="flex-1">Inbox</span>
                <span v-if="!sidebarCollapsed && unreadCount > 0" class="w-5 h-5 bg-blue-500 text-white text-[10px] font-bold rounded-full flex items-center justify-center flex-shrink-0">
                  {{ unreadCount > 9 ? '9+' : unreadCount }}
                </span>
              </router-link>
              <button v-if="!sidebarCollapsed && unreadCount > 0" @click="markAllRead"
                class="w-5 h-5 text-slate-600 hover:text-slate-300 transition-colors flex-shrink-0 ml-1" title="Mark all read">
                <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                </svg>
              </button>
            </div>
          </div>

          <!-- Divider + Group: Registers -->
          <div v-if="!sidebarCollapsed" class="text-[10px] text-slate-600 uppercase tracking-wider font-medium px-2 pt-4 pb-1">Registers</div>
          <div v-else class="border-t border-slate-800 my-2"></div>
          <div class="space-y-0.5">
            <router-link v-if="hasOrg" :to="orgPath('/risks')"
              class="flex items-center gap-2.5 py-1.5 text-sm rounded-lg transition-colors"
              :class="[route.path.startsWith(orgPath('/risks')) ? 'bg-slate-800 text-white font-medium' : 'text-slate-400 hover:text-white hover:bg-slate-800/50', sidebarCollapsed ? 'px-2.5 justify-center' : 'px-2']"
              :title="sidebarCollapsed ? 'Risks' : undefined">
              <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
              </svg>
              <span v-if="!sidebarCollapsed">Risks</span>
            </router-link>
            <router-link v-if="hasOrg" :to="orgPath('/assets')"
              class="flex items-center gap-2.5 py-1.5 text-sm rounded-lg transition-colors"
              :class="[route.path.startsWith(orgPath('/assets')) ? 'bg-slate-800 text-white font-medium' : 'text-slate-400 hover:text-white hover:bg-slate-800/50', sidebarCollapsed ? 'px-2.5 justify-center' : 'px-2']"
              :title="sidebarCollapsed ? 'Assets' : undefined">
              <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M21.75 17.25v-.228a4.5 4.5 0 00-.12-1.03l-2.268-9.64a3.375 3.375 0 00-3.285-2.602H7.923a3.375 3.375 0 00-3.285 2.602l-2.268 9.64a4.5 4.5 0 00-.12 1.03v.228m19.5 0a3 3 0 01-3 3H5.25a3 3 0 01-3-3m19.5 0a3 3 0 00-3-3H5.25a3 3 0 00-3 3m16.5 0h.008v.008h-.008v-.008zm-3 0h.008v.008h-.008v-.008z" />
              </svg>
              <span v-if="!sidebarCollapsed">Assets</span>
            </router-link>
            <router-link v-if="hasOrg" :to="orgPath('/suppliers')"
              class="flex items-center gap-2.5 py-1.5 text-sm rounded-lg transition-colors"
              :class="[route.path.startsWith(orgPath('/suppliers')) ? 'bg-slate-800 text-white font-medium' : 'text-slate-400 hover:text-white hover:bg-slate-800/50', sidebarCollapsed ? 'px-2.5 justify-center' : 'px-2']"
              :title="sidebarCollapsed ? 'Suppliers' : undefined">
              <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M8.25 18.75a1.5 1.5 0 01-3 0m3 0a1.5 1.5 0 00-3 0m3 0h6m-9 0H3.375a1.125 1.125 0 01-1.125-1.125V14.25m17.25 4.5a1.5 1.5 0 01-3 0m3 0a1.5 1.5 0 00-3 0m3 0H6.375c-.621 0-1.125-.504-1.125-1.125V14.25m17.25 0V6.375c0-.621-.504-1.125-1.125-1.125H15.75m0 0v3.375c0 .621-.504 1.125-1.125 1.125H12m3.75-4.5V3.375c0-.621-.504-1.125-1.125-1.125H8.25m0 0v3.375c0 .621.504 1.125 1.125 1.125H12" />
              </svg>
              <span v-if="!sidebarCollapsed">Suppliers</span>
            </router-link>
            <router-link v-if="hasOrg" :to="orgPath('/legal')"
              class="flex items-center gap-2.5 py-1.5 text-sm rounded-lg transition-colors"
              :class="[route.path.startsWith(orgPath('/legal')) ? 'bg-slate-800 text-white font-medium' : 'text-slate-400 hover:text-white hover:bg-slate-800/50', sidebarCollapsed ? 'px-2.5 justify-center' : 'px-2']"
              :title="sidebarCollapsed ? 'Legal' : undefined">
              <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 3v17.25m0 0c-1.472 0-2.882.265-4.185.75M12 20.25c1.472 0 2.882.265 4.185.75M18.75 4.97A48.416 48.416 0 0012 4.5c-2.291 0-4.545.16-6.75.47m13.5 0c1.01.143 2.01.317 3 .52m-3-.52l2.62 10.726c.122.499-.106 1.028-.589 1.202a5.988 5.988 0 01-2.031.352 5.988 5.988 0 01-2.031-.352c-.483-.174-.711-.703-.59-1.202L18.75 4.971zm-16.5.52c.99-.203 1.99-.377 3-.52m0 0l2.62 10.726c.122.499-.106 1.028-.589 1.202a5.989 5.989 0 01-2.031.352 5.989 5.989 0 01-2.031-.352c-.483-.174-.711-.703-.59-1.202L5.25 4.971z" />
              </svg>
              <span v-if="!sidebarCollapsed">Legal</span>
            </router-link>
            <router-link v-if="hasOrg" :to="orgPath('/systems')"
              class="flex items-center gap-2.5 py-1.5 text-sm rounded-lg transition-colors"
              :class="[route.path.startsWith(orgPath('/systems')) ? 'bg-slate-800 text-white font-medium' : 'text-slate-400 hover:text-white hover:bg-slate-800/50', sidebarCollapsed ? 'px-2.5 justify-center' : 'px-2']"
              :title="sidebarCollapsed ? 'Systems' : undefined">
              <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M8.25 3v1.5M4.5 8.25H3m18 0h-1.5M4.5 12H3m18 0h-1.5m-15 3.75H3m18 0h-1.5M8.25 19.5V21M12 3v1.5m0 15V21m3.75-18v1.5m0 15V21m-9-1.5h10.5a2.25 2.25 0 002.25-2.25V6.75a2.25 2.25 0 00-2.25-2.25H6.75A2.25 2.25 0 004.5 6.75v10.5a2.25 2.25 0 002.25 2.25zm.75-12h9v9h-9v-9z" />
              </svg>
              <span v-if="!sidebarCollapsed">Systems</span>
            </router-link>
          </div>

          <!-- Divider + Group: Operations -->
          <div v-if="!sidebarCollapsed" class="text-[10px] text-slate-600 uppercase tracking-wider font-medium px-2 pt-4 pb-1">Operations</div>
          <div v-else class="border-t border-slate-800 my-2"></div>
          <div class="space-y-0.5">
            <router-link v-if="hasOrg" :to="orgPath('/objectives')"
              class="flex items-center gap-2.5 py-1.5 text-sm rounded-lg transition-colors"
              :class="[route.path.startsWith(orgPath('/objectives')) ? 'bg-slate-800 text-white font-medium' : 'text-slate-400 hover:text-white hover:bg-slate-800/50', sidebarCollapsed ? 'px-2.5 justify-center' : 'px-2']"
              :title="sidebarCollapsed ? 'Objectives' : undefined">
              <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M3 3v1.5M3 21v-6m0 0l2.77-.693a9 9 0 016.208.682l.108.054a9 9 0 006.086.71l3.114-.732a48.524 48.524 0 01-.005-10.499l-3.11.732a9 9 0 01-6.085-.711l-.108-.054a9 9 0 00-6.208-.682L3 4.5M3 15V4.5" />
              </svg>
              <span v-if="!sidebarCollapsed">Objectives</span>
            </router-link>
            <router-link v-if="hasOrg" :to="orgPath('/programs')"
              class="flex items-center gap-2.5 py-1.5 text-sm rounded-lg transition-colors"
              :class="[route.path.startsWith(orgPath('/programs')) ? 'bg-slate-800 text-white font-medium' : 'text-slate-400 hover:text-white hover:bg-slate-800/50', sidebarCollapsed ? 'px-2.5 justify-center' : 'px-2']"
              :title="sidebarCollapsed ? 'Programs' : undefined">
              <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
              </svg>
              <span v-if="!sidebarCollapsed">Programs</span>
            </router-link>
            <router-link v-if="hasOrg" :to="orgPath('/audit')"
              class="flex items-center gap-2.5 py-1.5 text-sm rounded-lg transition-colors"
              :class="[route.path.startsWith(orgPath('/audit')) ? 'bg-slate-800 text-white font-medium' : 'text-slate-400 hover:text-white hover:bg-slate-800/50', sidebarCollapsed ? 'px-2.5 justify-center' : 'px-2']"
              :title="sidebarCollapsed ? 'Audit' : undefined">
              <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M11.35 3.836c-.065.21-.1.433-.1.664 0 .414.336.75.75.75h4.5a.75.75 0 00.75-.75 2.25 2.25 0 00-.1-.664m-5.8 0A2.251 2.251 0 0113.5 2.25H15a2.25 2.25 0 011.65 1.586m-5.8 0c-.376.023-.75.05-1.124.08C9.095 4.01 8.25 4.973 8.25 6.108V8.25m8.9-4.414c.376.023.75.05 1.124.08 1.131.094 1.976 1.057 1.976 2.192V16.5A2.25 2.25 0 0118 18.75h-2.25m-7.5-10.5H4.875c-.621 0-1.125.504-1.125 1.125v11.25c0 .621.504 1.125 1.125 1.125h9.75c.621 0 1.125-.504 1.125-1.125V18.75m-7.5-10.5h6.375c.621 0 1.125.504 1.125 1.125v9.375m-8.25-3l1.5 1.5 3-3.75" />
              </svg>
              <span v-if="!sidebarCollapsed">Audit</span>
            </router-link>
            <router-link v-if="hasOrg" :to="orgPath('/corrective-actions')"
              class="flex items-center gap-2.5 py-1.5 text-sm rounded-lg transition-colors"
              :class="[route.path.startsWith(orgPath('/corrective-actions')) ? 'bg-slate-800 text-white font-medium' : 'text-slate-400 hover:text-white hover:bg-slate-800/50', sidebarCollapsed ? 'px-2.5 justify-center' : 'px-2']"
              :title="sidebarCollapsed ? 'Corrective Actions' : undefined">
              <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M11.42 15.17l-5.1-5.1m0 0L3.44 12.95m2.88-2.88L3.44 7.19m8.58 10.86l5.1-5.1m0 0l2.88 2.88m-2.88-2.88l2.88-2.88M3.75 7.5h16.5" />
              </svg>
              <span v-if="!sidebarCollapsed">Corrective Actions</span>
            </router-link>
            <router-link v-if="hasOrg" :to="orgPath('/incidents')"
              class="flex items-center gap-2.5 py-1.5 text-sm rounded-lg transition-colors"
              :class="[route.path.startsWith(orgPath('/incidents')) ? 'bg-slate-800 text-white font-medium' : 'text-slate-400 hover:text-white hover:bg-slate-800/50', sidebarCollapsed ? 'px-2.5 justify-center' : 'px-2']"
              :title="sidebarCollapsed ? 'Incidents' : undefined">
              <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M11.25 11.25l.041-.02a.75.75 0 011.063.852l-.708 2.836a.75.75 0 001.063.853l.041-.021M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9-3.75h.008v.008H12V8.25z" />
              </svg>
              <span v-if="!sidebarCollapsed">Incidents</span>
            </router-link>
            <router-link v-if="hasOrg" :to="orgPath('/changes')"
              class="flex items-center gap-2.5 py-1.5 text-sm rounded-lg transition-colors"
              :class="[route.path.startsWith(orgPath('/changes')) ? 'bg-slate-800 text-white font-medium' : 'text-slate-400 hover:text-white hover:bg-slate-800/50', sidebarCollapsed ? 'px-2.5 justify-center' : 'px-2']"
              :title="sidebarCollapsed ? 'Change Management' : undefined">
              <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M7.5 21L3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5" />
              </svg>
              <span v-if="!sidebarCollapsed">Change Management</span>
            </router-link>
            <router-link v-if="hasOrg" :to="orgPath('/tasks')"
              class="flex items-center gap-2.5 py-1.5 text-sm rounded-lg transition-colors"
              :class="[route.path.startsWith(orgPath('/tasks')) ? 'bg-slate-800 text-white font-medium' : 'text-slate-400 hover:text-white hover:bg-slate-800/50', sidebarCollapsed ? 'px-2.5 justify-center' : 'px-2']"
              :title="sidebarCollapsed ? 'Tasks' : undefined">
              <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4" />
              </svg>
              <span v-if="!sidebarCollapsed">Tasks</span>
            </router-link>
          </div>

          <!-- Divider + Group: Admin (conditional) -->
          <template v-if="currentUserData?.role === 'admin'">
            <div v-if="!sidebarCollapsed" class="text-[10px] text-slate-600 uppercase tracking-wider font-medium px-2 pt-4 pb-1">Admin</div>
            <div v-else class="border-t border-slate-800 my-2"></div>
            <div class="space-y-0.5">
              <router-link v-if="hasOrg" :to="orgPath('/admin')"
                class="flex items-center gap-2.5 py-1.5 text-sm rounded-lg transition-colors"
                :class="[route.path.startsWith(orgPath('/admin')) ? 'bg-slate-800 text-white font-medium' : 'text-slate-400 hover:text-white hover:bg-slate-800/50', sidebarCollapsed ? 'px-2.5 justify-center' : 'px-2']"
                :title="sidebarCollapsed ? 'Admin' : undefined">
                <svg class="w-4 h-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.324.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.24-.438.613-.431.992a6.759 6.759 0 010 .255c-.007.378.138.75.43.99l1.005.828c.424.35.534.954.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.57 6.57 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.941-1.11.941h-2.594c-.55 0-1.02-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.992a6.932 6.932 0 010-.255c.007-.378-.138-.75-.43-.99l-1.004-.828a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.087.22-.128.332-.183.582-.495.644-.869l.214-1.281z" />
                  <path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                </svg>
                <span v-if="!sidebarCollapsed">Admin</span>
              </router-link>
            </div>
          </template>
        </nav>

        <!-- Collapse/Expand button -->
        <button @click="sidebarCollapsed = !sidebarCollapsed"
          class="flex items-center justify-center py-3 border-t border-slate-800 text-slate-500 hover:text-slate-300 hover:bg-slate-800/50 transition-colors flex-shrink-0"
          :title="sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'">
          <svg class="w-4 h-4 transition-transform duration-200" :class="{ 'rotate-180': sidebarCollapsed }" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M18.75 19.5l-7.5-7.5 7.5-7.5m-6 15L5.25 12l7.5-7.5" />
          </svg>
        </button>
      </aside>

      <!-- Notification dropdown -->
      <div
        v-if="showNotifications"
        class="notifications-panel fixed right-4 top-14 w-80 max-w-[calc(100vw-2rem)] bg-slate-900 border border-slate-800 rounded-xl shadow-xl z-50 overflow-hidden"
      >
        <div class="flex items-center justify-between px-4 py-3 border-b border-slate-800">
          <span class="text-sm font-semibold text-slate-200">Notifications</span>
          <button
            v-if="unreadCount > 0"
            @click="markAllRead"
            class="text-xs text-blue-400 hover:text-blue-300"
          >
            Mark all read
          </button>
        </div>
        <div class="max-h-72 overflow-y-auto divide-y divide-slate-800">
          <div v-if="notifications.length === 0" class="px-4 py-6 text-center text-xs text-slate-600">
            No notifications
          </div>
          <div
            v-for="n in notifications"
            :key="n.id"
            @click="markRead(n)"
            class="px-4 py-3 hover:bg-slate-800/50 transition-colors cursor-pointer"
            :class="n.read ? 'opacity-50' : ''"
          >
            <div class="text-sm text-slate-300 flex items-center gap-1.5">
              <span class="flex-1">{{ n.message || n.title }}</span>
              <svg v-if="n.link" class="w-3 h-3 text-slate-600 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M8.25 4.5l7.5 7.5-7.5 7.5" />
              </svg>
            </div>
            <div class="text-[10px] text-slate-600 mt-1">{{ formatNotifDate(n.created_at) }}</div>
          </div>
        </div>
      </div>

      <!-- Click-away overlay for notifications -->
      <div
        v-if="showNotifications"
        @click="showNotifications = false"
        class="fixed inset-0 z-40"
      />

      <!-- Right side: header + content -->
      <div class="flex-1 min-w-0 min-h-screen flex flex-col transition-all duration-200" :class="isMobile ? 'ml-0' : (sidebarCollapsed ? 'ml-14' : 'ml-60')">
        <!-- Top header bar -->
        <header class="h-14 bg-slate-900/50 border-b border-slate-800 flex items-center px-4 sm:px-6 sticky top-0 z-40 backdrop-blur-sm">
          <!-- Mobile menu button -->
          <button v-if="isMobile" @click="sidebarMobileOpen = !sidebarMobileOpen" class="p-1.5 mr-3 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800">
            <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6.75h16.5M3.75 12h16.5m-16.5 5.25h16.5" />
            </svg>
          </button>
          <GlobalSearch ref="globalSearchRef" />
          <div class="flex-1" />
          <div class="flex items-center gap-3">
            <!-- Notifications -->
            <button v-if="hasOrg" @click="showNotifications = !showNotifications" class="notifications-panel relative p-2 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800 transition-colors">
              <svg class="w-4.5 h-4.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
              </svg>
              <span v-if="unreadCount > 0" class="absolute top-0.5 right-0.5 w-4 h-4 bg-blue-500 text-white text-[10px] font-bold rounded-full flex items-center justify-center">
                {{ unreadCount > 9 ? '9+' : unreadCount }}
              </span>
            </button>

            <!-- Org switcher -->
            <div v-if="hasOrg" class="relative org-switcher">
              <button @click="showOrgMenu = !showOrgMenu" class="flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm hover:bg-slate-800 transition-colors border border-slate-800">
                <div class="w-5 h-5 rounded bg-blue-600/20 text-blue-400 flex items-center justify-center text-[10px] font-bold flex-shrink-0">{{ (orgName || orgSlug || '?').charAt(0).toUpperCase() }}</div>
                <span class="text-slate-300 font-medium">{{ orgSlug }}</span>
                <svg class="w-3 h-3 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5"><path stroke-linecap="round" stroke-linejoin="round" d="M19.5 8.25l-7.5 7.5-7.5-7.5"/></svg>
              </button>
              <!-- Org dropdown -->
              <div v-if="showOrgMenu" @click.stop class="absolute right-0 top-full mt-2 w-72 bg-slate-900 border border-slate-700 rounded-xl shadow-xl z-50 py-1">
                <div class="px-4 py-2.5 border-b border-slate-800">
                  <div class="text-[10px] text-slate-500 uppercase tracking-wider mb-1">Current</div>
                  <div class="text-sm font-medium text-white">{{ orgName }}</div>
                  <div class="text-xs text-slate-500">{{ currentUserData?.organization_slug }}</div>
                </div>
                <div v-if="currentUserData?.role === 'admin'" class="py-1 border-b border-slate-800">
                  <router-link :to="orgPath('/admin/settings')" @click="showOrgMenu = false" class="flex items-center gap-2 px-4 py-2 text-sm text-slate-300 hover:text-white hover:bg-slate-800/50 transition-colors">
                    <svg class="w-4 h-4 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/><path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/></svg>
                    Organization settings
                  </router-link>
                </div>
                <div v-if="otherOrgs.length > 0" class="py-1 border-b border-slate-800">
                  <div class="px-4 py-1 text-[10px] text-slate-500 uppercase tracking-wider">Switch to</div>
                  <button v-for="org in otherOrgs" :key="org.slug" @click="switchOrg(org)" class="w-full px-4 py-2 text-left text-sm text-slate-300 hover:text-white hover:bg-slate-800/50 transition-colors flex items-center justify-between">
                    <span>{{ org.name }}</span>
                    <span class="text-[10px] px-1.5 py-0.5 rounded-full" :class="org.role === 'admin' ? 'bg-purple-500/20 text-purple-300' : 'bg-slate-500/20 text-slate-400'">{{ org.role }}</span>
                  </button>
                </div>
                <div v-if="!subdomainBound" class="border-t border-slate-800 py-1">
                  <router-link to="/organizations" @click="showOrgMenu = false" class="flex items-center gap-2 px-4 py-2 text-sm text-slate-400 hover:text-white hover:bg-slate-800/50 transition-colors">
                    <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M7.5 21L3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5"/></svg>
                    Switch organization
                  </router-link>
                </div>
              </div>
            </div>

            <!-- User avatar + dropdown -->
            <div class="relative user-menu">
              <button @click="showUserMenu = !showUserMenu" class="flex items-center gap-2 pl-2 pr-1 py-1 rounded-lg hover:bg-slate-800 transition-colors">
                <span class="text-sm text-slate-300 hidden sm:block">{{ displayName }}</span>
                <div class="w-7 h-7 rounded-full flex items-center justify-center text-xs font-bold text-white flex-shrink-0"
                  :class="currentUserData?.role === 'admin' ? 'bg-purple-600' : currentUserData?.role === 'manager' ? 'bg-blue-600' : 'bg-slate-600'">
                  {{ displayName?.charAt(0)?.toUpperCase() }}
                </div>
              </button>
              <!-- User dropdown -->
              <div v-if="showUserMenu" @click.stop class="absolute right-0 top-full mt-2 w-64 bg-slate-900 border border-slate-700 rounded-xl shadow-xl z-50 py-2">
                <div class="px-4 py-2 border-b border-slate-800">
                  <div class="text-sm font-medium text-white">{{ displayName }}</div>
                  <div class="text-xs text-slate-500">{{ currentUserData?.email }}</div>
                  <span v-if="hasOrg && currentUserData?.role" class="inline-block mt-1 text-[10px] font-semibold px-1.5 py-0.5 rounded-full" :class="roleBadgeClass(currentUserData.role)">{{ currentUserData.role }}</span>
                </div>
                <div class="py-1">
                  <router-link :to="orgSlug ? orgPath('/settings') : '/organizations'" @click="showUserMenu = false" class="flex items-center gap-2 px-4 py-2 text-sm text-slate-400 hover:text-white hover:bg-slate-800/50 transition-colors">
                    <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/><path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/></svg>
                    Settings
                  </router-link>
                  <button @click="logout" class="w-full flex items-center gap-2 px-4 py-2 text-sm text-slate-400 hover:text-white hover:bg-slate-800/50 transition-colors">
                    <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"/></svg>
                    Sign out
                  </button>
                </div>
              </div>
            </div>

            <!-- Click-away overlays -->
          </div>
        </header>

        <!-- Main content -->
        <main class="flex-1">
          <router-view />
        </main>

        <!-- Footer -->
        <footer v-if="termsUrl || privacyUrl || showPoweredBy" class="px-8 py-4 text-center text-xs text-slate-600 border-t border-slate-800/50">
          <a v-if="privacyUrl" :href="privacyUrl" target="_blank" class="hover:text-slate-400 transition-colors">Privacy Policy</a>
          <span v-if="(termsUrl && privacyUrl)" class="mx-2">&middot;</span>
          <a v-if="termsUrl" :href="termsUrl" target="_blank" class="hover:text-slate-400 transition-colors">Terms of Service</a>
          <span v-if="showPoweredBy && (termsUrl || privacyUrl)" class="mx-2">&middot;</span>
          <a v-if="showPoweredBy" href="https://isms.sh" target="_blank" rel="noopener" class="hover:text-slate-400 transition-colors">Powered by isms.sh</a>
        </footer>
      </div>
    </div>
  </div>

  <!-- Toast notifications -->
  <Teleport to="body">
    <div class="fixed bottom-4 right-4 z-[100] space-y-2 max-w-sm">
      <transition-group name="toast">
        <div v-for="t in toasts" :key="t.id"
          class="px-4 py-3 rounded-lg shadow-xl text-sm font-medium cursor-pointer backdrop-blur-sm border"
          :class="{
            'bg-red-950/90 text-red-300 border-red-800': t.type === 'error',
            'bg-emerald-950/90 text-emerald-300 border-emerald-800': t.type === 'success',
            'bg-amber-950/90 text-amber-300 border-amber-800': t.type === 'warning',
          }"
          @click="dismissToast(t.id)">
          {{ t.message }}
        </div>
      </transition-group>
    </div>
  </Teleport>

  <!-- Global confirm dialog -->
  <Teleport to="body">
    <div v-if="confirmDialog.visible.value" class="fixed inset-0 z-[200] flex items-center justify-center">
      <div class="absolute inset-0 bg-black/60" @click="confirmDialog.onCancel()"></div>
      <div class="relative bg-slate-900 border border-slate-700 rounded-xl shadow-2xl px-6 py-5 max-w-sm w-full mx-4 space-y-4">
        <p class="text-sm text-slate-200">{{ confirmDialog.message.value }}</p>
        <div class="flex justify-end gap-2">
          <button @click="confirmDialog.onCancel()" class="px-3 py-1.5 text-xs text-slate-400 hover:text-slate-200">{{ confirmDialog.cancelLabel.value }}</button>
          <button @click="confirmDialog.onConfirm()"
            class="px-3 py-1.5 text-xs font-medium text-white rounded-lg transition-colors"
            :class="confirmDialog.variant.value === 'danger' ? 'bg-red-600 hover:bg-red-500' : 'bg-amber-600 hover:bg-amber-500'">
            {{ confirmDialog.confirmLabel.value }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from './api'
import { useToast } from './composables/useToast'
import { useConfirm } from './composables/useConfirm'

const confirmDialog = useConfirm()
import GlobalSearch from './components/GlobalSearch.vue'
import { useSession } from './composables/useSession'
import { useNotifications } from './composables/useNotifications'
import { useCurrentOrg, currentOrgPath, registerRouter, isSubdomainMode } from './composables/useCurrentOrg'

const { toasts, dismiss: dismissToast } = useToast()

const route = useRoute()
const router = useRouter()

// Session composable — user data, branding, orgs, JWT refresh
const {
  user, currentUserData, userOrgs,
  orgName, logoUrl, logoError, termsUrl, privacyUrl, showPoweredBy,
  openReviewCount,
  hasOrg, otherOrgs, displayName,
  startRefreshTimer, stopRefreshTimer,
  loadUserData, loadBranding, loadReviewCount,
  logout: sessionLogout, switchOrg: sessionSwitchOrg,
} = useSession()

// Notifications composable
const {
  notifications, unreadCount, showNotifications,
  loadNotifications, loadUnreadCount, markRead: notifMarkRead, markAllRead: notifMarkAllRead,
  formatNotifDate,
} = useNotifications()

// On a tenant subdomain the org is bound — no switcher, no picker, no
// "create new org" link. Used to hide multi-org UI surfaces below.
const subdomainBound = isSubdomainMode()

// Local UI state (not worth extracting — template-only concerns)
const globalSearchRef = ref(null)
const showUserMenu = ref(false)
const showOrgMenu = ref(false)
const sidebarCollapsed = ref(window.innerWidth < 1024)
const sidebarMobileOpen = ref(false)
const isMobile = ref(window.innerWidth < 768)
const editingName = ref(false)
const editNameValue = ref('')

// Auto-collapse sidebar on resize
if (typeof window !== 'undefined') {
  window.addEventListener('resize', () => {
    isMobile.value = window.innerWidth < 768
    if (window.innerWidth < 1024) sidebarCollapsed.value = true
  })
}

// Route-aware computed (needs route access, so stays in App.vue).
// Org slug comes from either the host subdomain or the :org route param.
const { orgSlug: routeOrgSlug, orgPath } = useCurrentOrg()
const orgSlug = computed(() => routeOrgSlug.value || currentUserData.value?.organization_slug || '')
const orgHomeLink = computed(() => orgSlug.value ? orgPath('/overview') : '/')
const isPublicRoute = computed(() =>
  route.path === '/login' ||
  route.path === '/signup' ||
  route.path === '/forgot-password' ||
  route.path === '/verify-email' ||
  route.path === '/' ||
  route.path === '/landing'
)

const nav = computed(() => {
  if (!hasOrg.value) return []
  const slug = currentUserData.value?.organization_slug
  if (!slug) return []

  const base = [
    { path: orgPath('/overview'), label: 'Overview' },
    { path: orgPath('/documents'), label: 'Documents' },
    { path: orgPath('/inbox'), label: 'Inbox' },
    { path: orgPath('/risks'), label: 'Risks' },
    { path: orgPath('/suppliers'), label: 'Suppliers' },
    { path: orgPath('/legal'), label: 'Legal' },
    { path: orgPath('/assets'), label: 'Assets' },
    { path: orgPath('/systems'), label: 'Systems' },
    { path: orgPath('/objectives'), label: 'Objectives' },
    { path: orgPath('/programs'), label: 'Programs' },
    { path: orgPath('/audit'), label: 'Audit' },
    { path: orgPath('/corrective-actions'), label: 'Corrective Actions' },
    { path: orgPath('/incidents'), label: 'Incidents' },
    { path: orgPath('/changes'), label: 'Change Management' },
    { path: orgPath('/tasks'), label: 'Tasks' },
    { path: orgPath('/reviews'), label: 'Reviews' },
  ]
  if (currentUserData.value?.role === 'admin') {
    base.push({ path: orgPath('/admin'), label: 'Admin' })
  }
  return base
})

// Thin wrappers that wire composable calls to local context (router, orgSlug, showUserMenu)
async function logout() {
  showUserMenu.value = false
  await sessionLogout(router)
}

async function switchOrg(org) {
  showOrgMenu.value = false
  await sessionSwitchOrg(org)
}

async function markRead(n) {
  await notifMarkRead(n, orgSlug.value, router)
}

async function markAllRead() {
  await notifMarkAllRead(user.value)
}

function roleBadgeClass(role) {
  const map = {
    admin: 'bg-purple-500/20 text-purple-300',
    manager: 'bg-blue-500/20 text-blue-300',
    contributor: 'bg-emerald-500/20 text-emerald-300',
    reader: 'bg-slate-500/20 text-slate-400',
  }
  return map[role] || 'bg-slate-500/20 text-slate-400'
}

function startEditName() {
  editNameValue.value = currentUserData.value?.name || displayName.value
  editingName.value = true
}

async function saveName() {
  const newName = editNameValue.value.trim()
  if (!newName) return
  try {
    await api.upsertUser({
      email: user.value,
      name: newName,
      role: currentUserData.value?.role || '',
    })
    if (currentUserData.value) {
      currentUserData.value.name = newName
    }
    editingName.value = false
  } catch (e) {
    console.error('Failed to update name:', e)
  }
}

// Orchestrate all data loading
async function loadAppData() {
  if (isPublicRoute.value || route.path === '/organizations') return
  await loadBranding()
  const ok = await loadUserData(route, router)
  if (!ok) return

  await loadUnreadCount()
  await loadReviewCount()
  await loadNotifications(user.value)

  startRefreshTimer(router)
}

// Make the router instance reachable from non-setup code (used by the
// global markdown-link click delegate below, which runs on raw DOM events).
registerRouter(router)

// Load on mount and when navigating away from login (after successful login)
onMounted(() => {
  loadAppData()
  // Close dropdowns on click outside
  document.addEventListener('click', (e) => {
    if (showOrgMenu.value && !e.target.closest('.org-switcher')) showOrgMenu.value = false
    if (showUserMenu.value && !e.target.closest('.user-menu')) showUserMenu.value = false
    if (showNotifications.value && !e.target.closest('.notifications-panel')) showNotifications.value = false
  })
  // Global click delegate for internal links inside rendered markdown
  // (and anywhere else in the app). Intercepts left-clicks without
  // modifier keys on absolute SPA paths and routes them through Vue
  // Router with the correct org prefix. Cmd-click / middle-click /
  // shift-click are NOT intercepted, so "open in new tab" still works.
  //
  // This replaces the per-view `html.replace(/href="\/documents\//...)` hack
  // and additionally handles cross-entity links (/risks/, /incidents/, ...).
  document.addEventListener('click', (e) => {
    if (e.defaultPrevented || e.button !== 0) return
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
    const a = e.target.closest && e.target.closest('a[href]')
    if (!a) return
    // Respect explicit target="_blank" / download links.
    const target = a.getAttribute('target')
    if (target && target !== '' && target !== '_self') return
    if (a.hasAttribute('download')) return
    const href = a.getAttribute('href') || ''
    // Only SPA-internal absolute paths. Skip external, protocol-relative,
    // hash anchors, mailto:, tel:, etc.
    if (!href.startsWith('/') || href.startsWith('//')) return
    // Only intercept if Vue Router actually has a route for this path.
    // Server-served paths (e.g. /docs Scalar UI, /api/openapi.yaml,
    // /healthz, /branding/...) must navigate natively — otherwise the SPA
    // captures the click, no route matches, and the auth guard kicks the
    // user to /login.
    const candidate = currentOrgPath(href)
    const resolved = router.resolve(candidate)
    if (!resolved.matched.length) return
    e.preventDefault()
    router.push(resolved.fullPath)
  })
  // Global click delegate for the copy buttons on rendered code blocks
  // (emitted by the shared markdown renderer in useRenderMd). Copies the raw
  // <code> textContent so the clipboard gets clean source, not span markup.
  document.addEventListener('click', (e) => {
    const btn = e.target.closest && e.target.closest('.copy-code-btn')
    if (!btn) return
    e.preventDefault()
    const code = btn.parentElement && btn.parentElement.querySelector('code')
    if (!code) return
    const text = code.textContent || ''
    const confirm = () => {
      btn.classList.add('copied')
      btn.textContent = 'Copied'
      setTimeout(() => { btn.classList.remove('copied'); btn.textContent = 'Copy' }, 1500)
    }
    // Clipboard API needs a secure context (https / localhost). Self-hosted
    // deployments on plain http (e.g. a LAN box) don't have it, so fall back
    // to the legacy execCommand path — the copy button must work everywhere.
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text).then(confirm).catch(() => {})
      return
    }
    try {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.top = '-9999px'
      document.body.appendChild(ta)
      ta.focus()
      ta.select()
      const ok = document.execCommand('copy')
      document.body.removeChild(ta)
      if (ok) confirm()
    } catch (_) { /* clipboard unavailable — silently no-op */ }
  })
  // Handle 401 from any API call — redirect to login. Skip when the user is
  // already on a public auth page (signup / forgot-password / verify-email /
  // login itself) so that a stale token doesn't kick them away from a flow
  // they're actively trying to complete.
  window.addEventListener('isms:unauthorized', () => {
    stopRefreshTimer()
    const publicAuthPaths = ['/login', '/signup', '/forgot-password', '/verify-email', '/']
    if (publicAuthPaths.includes(route.path)) return
    router.push('/login')
  })
})
onUnmounted(stopRefreshTimer)
watch(() => route.path, (newPath, oldPath) => {
  // Reload app data when navigating from a public/landing path into an org route.
  // Includes `/` because Vue Router's initial render hits apex before beforeEach
  // redirects to /overview on subdomain hosts — that first onMounted call returns
  // early (isPublicRoute), so we need this watcher to fire loadAppData on the hop.
  const publicPaths = ['/login', '/organizations', '/']
  if (publicPaths.includes(oldPath) && !publicPaths.includes(newPath)) {
    loadAppData()
  }
  // Also reload when org changes in URL (path-based mode only — on subdomain
  // mode the org is bound to the host so it never changes mid-session).
  if (route.params.org && route.params.org !== currentUserData.value?.organization_slug) {
    loadAppData()
  }
  // Auto-mark notifications read when visiting inbox or reviews
  if (newPath.includes('/inbox') || newPath.includes('/reviews')) {
    if (unreadCount.value > 0) {
      markAllRead()
    }
  }
})
</script>

<style>
/* Brand color integration — uses --brand-color CSS variable set from admin branding config.
   Falls back to Tailwind blue-500 (#3b82f6) when no brand color is configured. */
:root {
  --brand-color: #3b82f6;
}

/* Org name in sidebar header */
.brand-org-name {
  color: var(--brand-color);
}

/* Active sidebar nav items get a left accent bar in the brand color */
aside nav a.router-link-active {
  border-left: 3px solid var(--brand-color);
  padding-left: calc(0.5rem - 3px);
}

/* Primary action buttons — override Tailwind bg-blue-600 with brand color */
button.bg-blue-600,
a.bg-blue-600 {
  background-color: var(--brand-color);
}
button.bg-blue-600:hover,
a.bg-blue-600:hover {
  background-color: var(--brand-color);
  filter: brightness(1.15);
}

/* Link-style actions that use blue-400 text (e.g. "Mark all read") */
button.text-blue-400,
a.text-blue-400 {
  color: var(--brand-color);
}
button.text-blue-400:hover,
a.text-blue-400:hover {
  filter: brightness(1.2);
}
</style>
