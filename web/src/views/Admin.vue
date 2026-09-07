<template>
  <div class="min-h-full">
    <!-- Loading -->
    <div v-if="pageLoading" class="flex items-center justify-center h-96">
      <div class="text-slate-400 text-sm">{{ t('admin.loading') }}</div>
    </div>

    <!-- Main content -->
    <div v-else class="max-w-5xl mx-auto px-8 py-10 space-y-6">
      <!-- Header -->
      <div>
        <h1 class="text-2xl font-bold text-slate-100 tracking-tight">{{ t('admin.title') }}</h1>
        <p class="text-sm text-slate-500 mt-1">{{ t('admin.subtitle') }}</p>
      </div>

      <!-- Tab bar -->
      <div class="flex gap-1 bg-slate-900 border border-slate-800 rounded-lg p-1">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          @click="switchTab(tab.key)"
          class="flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-md transition-colors"
          :class="activeTab === tab.key
            ? 'bg-slate-800 text-white'
            : 'text-slate-500 hover:text-slate-300'"
        >
          {{ tab.label }}
        </button>
      </div>

      <!-- ===================== MEMBERS TAB ===================== -->
      <template v-if="activeTab === 'members'">
        <div class="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
          <table class="w-full">
            <thead>
              <tr class="border-b border-slate-800">
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('admin.members.header.name') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('admin.members.header.email') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('admin.members.header.role') }}</th>
                <th class="text-right px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('admin.members.header.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800">
              <tr v-for="member in members" :key="member.id || member.email" class="hover:bg-slate-800/50 transition-colors">
                <td class="px-5 py-3">
                  <div class="flex items-center gap-3">
                    <div class="w-8 h-8 rounded-full flex items-center justify-center text-xs font-bold text-white flex-shrink-0"
                      :class="member.role === 'admin' ? 'bg-purple-600' : member.role === 'manager' ? 'bg-blue-600' : 'bg-slate-600'">
                      {{ (member.name || member.email || '?').charAt(0).toUpperCase() }}
                    </div>
                    <span class="text-sm text-slate-200">{{ member.name || '-' }}</span>
                    <span v-if="member.is_agent" class="ml-1.5 px-1.5 py-0.5 rounded text-[9px] font-semibold bg-purple-500/15 text-purple-400">{{ t('admin.members.agent_badge') }}</span>
                    <span v-if="!member.active" class="ml-1.5 px-1.5 py-0.5 rounded text-[9px] font-semibold bg-amber-500/15 text-amber-400">{{ t('admin.members.pending_badge') }}</span>
                  </div>
                </td>
                <td class="px-5 py-3 text-sm text-slate-400">{{ member.email }}</td>
                <td class="px-5 py-3">
                  <select
                    :value="member.role"
                    @change="updateMemberRole(member, $event.target.value)"
                    class="bg-slate-800 border border-slate-700 rounded-md px-2 py-1 text-sm text-slate-300 focus:outline-none focus:border-blue-500"
                  >
                    <option v-for="r in roleOptions" :key="r.value" :value="r.value">{{ r.label }}</option>
                  </select>
                </td>
                <td class="px-5 py-3 text-right">
                  <button v-if="!member.active" @click="resendInvite(member)"
                    class="text-xs text-blue-400 hover:text-blue-300 px-2 py-1 rounded hover:bg-blue-500/10 transition-colors mr-1">
                    {{ t('admin.members.resend_invite') }}
                  </button>
                  <button @click="removeMember(member)"
                    class="text-xs text-red-400 hover:text-red-300 px-2 py-1 rounded hover:bg-red-500/10 transition-colors">
                    {{ t('admin.members.remove') }}
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
          <div v-if="members.length === 0" class="px-5 py-12 text-center text-sm text-slate-600">
            {{ t('admin.members.empty') }}
          </div>
        </div>
        <div v-if="membersMsg" class="text-xs" :class="membersError ? 'text-red-400' : 'text-emerald-400'">{{ membersMsg }}</div>
      </template>

      <!-- ===================== INVITE TAB ===================== -->
      <template v-if="activeTab === 'invite'">
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-6 max-w-lg">
          <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider mb-5">{{ t('admin.invite.title') }}</h2>
          <form @submit.prevent="sendInvite" class="space-y-4">
            <div>
              <label class="block text-xs text-slate-500 mb-1">{{ t('admin.invite.email') }}</label>
              <input v-model="inviteEmail" type="email" required
                class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500" :placeholder="t('admin.invite.email_placeholder')" />
            </div>
            <div>
              <label class="block text-xs text-slate-500 mb-1">{{ t('admin.invite.name') }}</label>
              <input v-model="inviteName" type="text"
                class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500" :placeholder="t('admin.invite.name_placeholder')" />
            </div>
            <div>
              <label class="block text-xs text-slate-500 mb-1">{{ t('admin.invite.role') }}</label>
              <select v-model="inviteRole"
                class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500">
                <option v-for="r in inviteRoleOptions" :key="r.value" :value="r.value">{{ r.label }}</option>
              </select>
            </div>
            <button type="submit" :disabled="inviting"
              class="px-5 py-2.5 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors disabled:opacity-50">
              {{ inviting ? t('admin.invite.sending') : t('admin.invite.submit') }}
            </button>
          </form>
          <div v-if="inviteMsg" class="text-xs mt-3" :class="inviteError ? 'text-red-400' : 'text-emerald-400'">{{ inviteMsg }}</div>
        </div>
      </template>

      <!-- ===================== API KEYS TAB (read-only audit view) ===================== -->
      <template v-if="activeTab === 'api-keys'">
        <div class="bg-slate-900/50 border border-slate-800 rounded-lg p-4 mb-4">
          <p class="text-sm text-slate-400">
            <i18n-t keypath="admin.api_keys.note" tag="span" scope="global">
              <template #settings><strong class="text-slate-300">{{ t('admin.api_keys.note_settings') }}</strong></template>
            </i18n-t>
          </p>
        </div>

        <div class="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
          <table class="w-full">
            <thead>
              <tr class="border-b border-slate-800">
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('admin.api_keys.header.name') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('admin.api_keys.header.user') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('admin.api_keys.header.permissions') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('admin.api_keys.header.created') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('admin.api_keys.header.last_used') }}</th>
                <th class="text-left px-5 py-3 text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('admin.api_keys.header.status') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-slate-800">
              <tr v-for="key in apiKeys" :key="key.id" class="hover:bg-slate-800/50 transition-colors">
                <td class="px-5 py-3 text-sm text-slate-200 font-medium">{{ key.name }}</td>
                <td class="px-5 py-3 text-sm text-slate-400">{{ key.user_email }}</td>
                <td class="px-5 py-3 text-sm text-slate-400">{{ permissionLabel(key.permissions) }}</td>
                <td class="px-5 py-3 text-sm text-slate-500">{{ formatDate(key.created_at) }}</td>
                <td class="px-5 py-3 text-sm text-slate-500">{{ key.last_used_at ? formatDate(key.last_used_at) : t('admin.api_keys.never') }}</td>
                <td class="px-5 py-3">
                  <span class="inline-flex items-center gap-1.5 text-xs font-medium"
                    :class="key.revoked_at ? 'text-red-400' : 'text-emerald-400'">
                    <span class="w-1.5 h-1.5 rounded-full" :class="key.revoked_at ? 'bg-red-400' : 'bg-emerald-400'"></span>
                    {{ key.revoked_at ? t('admin.api_keys.revoked') : t('admin.api_keys.active') }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
          <div v-if="apiKeys.length === 0" class="px-5 py-12 text-center text-sm text-slate-600">
            {{ t('admin.api_keys.empty') }}
          </div>
        </div>
      </template>

      <!-- ===================== AUTHENTICATION TAB ===================== -->
      <template v-if="activeTab === 'authentication'">
        <!-- Existing providers -->
        <div v-for="provider in oidcProviders" :key="provider.id" class="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 rounded-lg flex items-center justify-center text-lg font-bold"
                :class="providerIconClass(provider.provider_name)">
                {{ (provider.display_name || provider.provider_name || '?').charAt(0).toUpperCase() }}
              </div>
              <div>
                <h3 class="text-sm font-semibold text-white">{{ provider.display_name || provider.provider_name }}</h3>
                <p class="text-xs text-slate-500">{{ provider.provider_name }}</p>
              </div>
            </div>
            <div class="flex items-center gap-3">
              <button @click="testProvider(provider)" :disabled="provider._testing"
                class="text-xs text-blue-400 hover:text-blue-300 px-2 py-1 rounded hover:bg-blue-500/10 transition-colors disabled:opacity-50">
                {{ provider._testing ? t('admin.oidc.testing') : t('admin.oidc.test') }}
              </button>
              <button @click="deleteProvider(provider)"
                class="text-xs text-red-400 hover:text-red-300 px-2 py-1 rounded hover:bg-red-500/10 transition-colors">
                {{ t('admin.oidc.delete') }}
              </button>
            </div>
          </div>

          <div class="grid grid-cols-2 gap-4">
            <div>
              <label class="block text-xs text-slate-500 mb-1">{{ t('admin.oidc.display_name') }}</label>
              <input v-model="provider.display_name" type="text"
                class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500" />
            </div>
            <div>
              <label class="block text-xs text-slate-500 mb-1">{{ t('admin.oidc.client_id') }}</label>
              <input v-model="provider.client_id" type="text"
                class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500" :placeholder="t('admin.oidc.client_id_placeholder')" />
            </div>
            <div>
              <label class="block text-xs text-slate-500 mb-1">{{ t('admin.oidc.client_secret') }}</label>
              <input v-model="provider.client_secret" type="password"
                class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500" :placeholder="t('admin.oidc.client_secret_placeholder')" />
            </div>
            <div>
              <label class="block text-xs text-slate-500 mb-1">{{ t('admin.oidc.discovery_url') }}</label>
              <input v-model="provider.discovery_url" type="text"
                class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500" :placeholder="t('admin.oidc.discovery_url_placeholder')" />
            </div>
            <div>
              <label class="block text-xs text-slate-500 mb-1">{{ t('admin.oidc.default_role') }}</label>
              <select v-model="provider.default_role"
                class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500">
                <option v-for="r in oidcRoleOptions" :key="r.value" :value="r.value">{{ r.label }}</option>
              </select>
            </div>
            <div class="flex items-end gap-6 pb-1">
              <label class="flex items-center gap-2 cursor-pointer">
                <input type="checkbox" v-model="provider.enabled" class="w-4 h-4 rounded bg-slate-800 border-slate-600 text-blue-500 focus:ring-blue-500/30" />
                <span class="text-sm text-slate-400">{{ t('admin.oidc.enabled') }}</span>
              </label>
              <label class="flex items-center gap-2 cursor-pointer">
                <input type="checkbox" v-model="provider.auto_add_members" class="w-4 h-4 rounded bg-slate-800 border-slate-600 text-blue-500 focus:ring-blue-500/30" />
                <span class="text-sm text-slate-400">{{ t('admin.oidc.auto_add') }}</span>
              </label>
            </div>
          </div>

          <div class="flex gap-2">
            <button @click="saveProvider(provider)" :disabled="provider._saving"
              class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm rounded-lg transition-colors disabled:opacity-50">
              {{ provider._saving ? t('common.state.saving') : t('common.action.save') }}
            </button>
          </div>
          <div v-if="provider._msg" class="text-xs" :class="provider._error ? 'text-red-400' : 'text-emerald-400'">{{ provider._msg }}</div>
        </div>

        <!-- Add new provider -->
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-6 space-y-4">
          <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider">{{ t('admin.oidc.add_title') }}</h2>

          <!-- Presets -->
          <div class="flex gap-2">
            <button @click="applyPreset('microsoft')"
              class="flex items-center gap-2 px-3 py-2 bg-[#2F2F2F] hover:bg-[#3b3b3b] border border-[#3b3b3b] rounded-lg text-sm text-white transition-colors">
              <svg class="w-4 h-4" viewBox="0 0 21 21" fill="none">
                <rect x="1" y="1" width="9" height="9" fill="#F25022"/>
                <rect x="11" y="1" width="9" height="9" fill="#7FBA00"/>
                <rect x="1" y="11" width="9" height="9" fill="#00A4EF"/>
                <rect x="11" y="11" width="9" height="9" fill="#FFB900"/>
              </svg>
              {{ t('admin.oidc.preset.microsoft') }}
            </button>
            <button @click="applyPreset('google')"
              class="flex items-center gap-2 px-3 py-2 bg-white hover:bg-gray-50 border border-gray-300 rounded-lg text-sm text-gray-700 transition-colors">
              <svg class="w-4 h-4" viewBox="0 0 24 24">
                <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 01-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z" fill="#4285F4"/>
                <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/>
                <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" fill="#FBBC05"/>
                <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/>
              </svg>
              {{ t('admin.oidc.preset.google') }}
            </button>
            <button @click="applyPreset('okta')"
              class="flex items-center gap-2 px-3 py-2 bg-slate-800 hover:bg-slate-700 border border-slate-700 rounded-lg text-sm text-slate-200 transition-colors">
              {{ t('admin.oidc.preset.okta') }}
            </button>
            <button @click="applyPreset('custom')"
              class="flex items-center gap-2 px-3 py-2 bg-slate-800 hover:bg-slate-700 border border-slate-700 rounded-lg text-sm text-slate-200 transition-colors">
              {{ t('admin.oidc.preset.custom') }}
            </button>
          </div>

          <div v-if="providerType !== null" class="space-y-4">
            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class="block text-xs text-slate-500 mb-1">{{ t('admin.oidc.provider_name') }}</label>
                <input v-model="newProvider.provider_name" type="text"
                  class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500" :placeholder="t('admin.oidc.provider_name_placeholder')" />
              </div>
              <div>
                <label class="block text-xs text-slate-500 mb-1">{{ t('admin.oidc.display_name') }}</label>
                <input v-model="newProvider.display_name" type="text"
                  class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500" :placeholder="t('admin.oidc.new_display_name_placeholder')" />
              </div>
              <div>
                <label class="block text-xs text-slate-500 mb-1">{{ t('admin.oidc.client_id') }}</label>
                <input v-model="newProvider.client_id" type="text"
                  class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500" :placeholder="t('admin.oidc.new_client_id_placeholder')" />
              </div>
              <div>
                <label class="block text-xs text-slate-500 mb-1">{{ t('admin.oidc.client_secret') }}</label>
                <input v-model="newProvider.client_secret" type="password"
                  class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500" :placeholder="t('admin.oidc.client_secret_placeholder')" />
              </div>
              <!-- Microsoft: Tenant ID -->
              <div v-if="providerType === 'microsoft'" class="col-span-2">
                <label class="block text-xs text-slate-500 mb-1">{{ t('admin.oidc.tenant_id') }}</label>
                <input v-model="tenantId" type="text"
                  class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500"
                  :placeholder="t('admin.oidc.tenant_id_placeholder')" />
                <p class="text-[10px] text-slate-600 mt-1">{{ t('admin.oidc.tenant_id_hint') }}</p>
              </div>
              <!-- Okta: Domain -->
              <div v-if="providerType === 'okta'" class="col-span-2">
                <label class="block text-xs text-slate-500 mb-1">{{ t('admin.oidc.okta_domain') }}</label>
                <input v-model="oktaDomain" type="text"
                  class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500"
                  :placeholder="t('admin.oidc.okta_domain_placeholder')" />
                <p class="text-[10px] text-slate-600 mt-1">{{ t('admin.oidc.okta_domain_hint') }}</p>
              </div>
              <!-- Google: hardcoded note -->
              <div v-if="providerType === 'google'" class="col-span-2 text-xs text-slate-500 bg-slate-800/50 border border-slate-700 rounded-lg px-3 py-2">
                <i18n-t keypath="admin.oidc.google_discovery_note" tag="span" scope="global">
                  <template #url><code class="text-slate-300 font-mono">https://accounts.google.com/.well-known/openid-configuration</code></template>
                </i18n-t>
              </div>
              <!-- Custom/Generic: raw discovery URL -->
              <div v-if="!providerType" class="col-span-2">
                <label class="block text-xs text-slate-500 mb-1">{{ t('admin.oidc.discovery_url') }}</label>
                <input v-model="newProvider.discovery_url" type="text"
                  class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500" :placeholder="t('admin.oidc.custom_discovery_placeholder')" />
              </div>
              <div>
                <label class="block text-xs text-slate-500 mb-1">{{ t('admin.oidc.default_role') }}</label>
                <select v-model="newProvider.default_role"
                  class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500">
                  <option v-for="r in oidcRoleOptions" :key="r.value" :value="r.value">{{ r.label }}</option>
                </select>
              </div>
              <div class="flex items-end gap-6 pb-1">
                <label class="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" v-model="newProvider.enabled" class="w-4 h-4 rounded bg-slate-800 border-slate-600 text-blue-500 focus:ring-blue-500/30" />
                  <span class="text-sm text-slate-400">{{ t('admin.oidc.enabled') }}</span>
                </label>
                <label class="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" v-model="newProvider.auto_add_members" class="w-4 h-4 rounded bg-slate-800 border-slate-600 text-blue-500 focus:ring-blue-500/30" />
                  <span class="text-sm text-slate-400">{{ t('admin.oidc.auto_add') }}</span>
                </label>
              </div>
            </div>
            <div class="flex gap-2">
              <button @click="addProvider" :disabled="addingProvider"
                class="px-5 py-2.5 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors disabled:opacity-50">
                {{ addingProvider ? t('admin.oidc.adding') : t('admin.oidc.add') }}
              </button>
              <button @click="resetNewProvider"
                class="px-4 py-2.5 text-sm text-slate-400 hover:text-slate-300 transition-colors">
                {{ t('common.action.cancel') }}
              </button>
            </div>
            <div v-if="newProviderMsg" class="text-xs" :class="newProviderError ? 'text-red-400' : 'text-emerald-400'">{{ newProviderMsg }}</div>
          </div>
        </div>
      </template>

      <!-- ===================== BRANDING TAB ===================== -->
      <template v-if="activeTab === 'branding'">
        <div class="bg-slate-900 border border-slate-800 rounded-xl p-6 max-w-lg space-y-5">
          <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider">{{ t('admin.branding.title') }}</h2>
          <div>
            <label class="block text-xs text-slate-500 mb-1">{{ t('admin.branding.name') }}</label>
            <input v-model="branding.branding_name" type="text"
              class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500"
              :placeholder="t('admin.branding.name_placeholder')" />
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1">{{ t('admin.branding.color') }}</label>
            <div class="flex items-center gap-3">
              <input v-model="branding.branding_color" type="color"
                class="w-10 h-10 rounded-lg border border-slate-700 bg-slate-800 cursor-pointer p-0.5" />
              <input v-model="branding.branding_color" type="text"
                class="flex-1 bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500 font-mono"
                placeholder="#3b82f6" />
              <div class="w-8 h-8 rounded-lg border border-slate-700" :style="{ backgroundColor: branding.branding_color }" />
            </div>
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1">{{ t('admin.branding.logo') }}</label>
            <div class="flex items-center gap-3 mb-2">
              <label class="px-3 py-1.5 bg-slate-800 hover:bg-slate-700 border border-slate-700 rounded-lg text-xs text-slate-300 cursor-pointer transition-colors">
                {{ t('admin.branding.upload') }}
                <input type="file" accept=".png,.svg" class="hidden" @change="uploadBrandingFile($event, 'logo')" />
              </label>
              <span v-if="uploadMsg.logo" class="text-[10px]" :class="uploadErr.logo ? 'text-red-400' : 'text-emerald-400'">{{ uploadMsg.logo }}</span>
            </div>
            <div class="text-[10px] text-slate-600">{{ t('admin.branding.logo_hint') }}</div>
            <div v-if="logoPreviewUrl" class="mt-3 p-4 bg-slate-800/50 border border-slate-700 rounded-lg">
              <div class="text-[10px] text-slate-600 mb-2">{{ t('admin.branding.preview') }}</div>
              <div class="flex items-center gap-6">
                <div>
                  <div class="text-[9px] text-slate-600 mb-1">{{ t('admin.branding.preview_sidebar') }}</div>
                  <div class="bg-slate-900 rounded-lg p-2 inline-block"><img :src="logoPreviewUrl" alt="" class="h-9 w-auto max-w-[8rem] object-contain" @error="logoPreviewUrl = ''" /></div>
                </div>
                <div>
                  <div class="text-[9px] text-slate-600 mb-1">{{ t('admin.branding.preview_login') }}</div>
                  <div class="bg-slate-900 rounded-lg p-2 inline-block"><img :src="logoPreviewUrl" alt="" class="h-14 w-auto max-w-[12rem] object-contain" @error="logoPreviewUrl = ''" /></div>
                </div>
              </div>
            </div>
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1">{{ t('admin.branding.favicon') }}</label>
            <div class="flex items-center gap-3 mb-2">
              <label class="px-3 py-1.5 bg-slate-800 hover:bg-slate-700 border border-slate-700 rounded-lg text-xs text-slate-300 cursor-pointer transition-colors">
                {{ t('admin.branding.upload') }}
                <input type="file" accept=".png,.ico,.svg" class="hidden" @change="uploadBrandingFile($event, 'favicon')" />
              </label>
              <span v-if="uploadMsg.favicon" class="text-[10px]" :class="uploadErr.favicon ? 'text-red-400' : 'text-emerald-400'">{{ uploadMsg.favicon }}</span>
            </div>
            <div class="text-[10px] text-slate-600">{{ t('admin.branding.favicon_hint') }}</div>
            <div v-if="faviconPreviewUrl" class="mt-3 flex items-center gap-3">
              <div class="bg-slate-800/50 border border-slate-700 rounded-lg p-2 inline-block"><img :src="faviconPreviewUrl" alt="" class="h-6 w-6 object-contain" @error="faviconPreviewUrl = ''" /></div>
              <span class="text-[10px] text-slate-600">{{ t('admin.branding.favicon_current') }}</span>
            </div>
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1">{{ t('admin.branding.footer') }}</label>
            <input v-model="branding.branding_footer" type="text"
              class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500"
              :placeholder="t('admin.branding.footer_placeholder')" />
          </div>

          <hr class="border-slate-800" />
          <h3 class="text-xs font-semibold text-slate-500 uppercase tracking-wider">{{ t('admin.branding.white_label') }}</h3>

          <div class="flex items-center gap-3">
            <label class="relative inline-flex items-center cursor-pointer">
              <input type="checkbox" v-model="showPoweredByToggle" class="sr-only peer" />
              <div class="w-9 h-5 bg-slate-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-600"></div>
            </label>
            <span class="text-sm text-slate-300">{{ t('admin.branding.show_powered_by') }}</span>
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1">{{ t('admin.branding.terms_url') }}</label>
            <input v-model="branding.terms_url" type="text"
              class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500"
              :placeholder="t('admin.branding.terms_url_placeholder')" />
            <p class="text-xs text-slate-600 mt-1">{{ t('admin.branding.terms_hint') }}</p>
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1">{{ t('admin.branding.privacy_url') }}</label>
            <input v-model="branding.privacy_url" type="text"
              class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500"
              :placeholder="t('admin.branding.privacy_url_placeholder')" />
            <p class="text-xs text-slate-600 mt-1">{{ t('admin.branding.privacy_hint') }}</p>
          </div>
          <button @click="saveBranding" :disabled="brandingSaving"
            class="px-5 py-2.5 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-lg transition-colors disabled:opacity-50">
            {{ brandingSaving ? t('admin.branding.saving') : t('admin.branding.save') }}
          </button>
          <div v-if="brandingMsg" class="text-xs" :class="brandingError ? 'text-red-400' : 'text-emerald-400'">{{ brandingMsg }}</div>
        </div>
      </template>

      <!-- ===================== POLICIES TAB ===================== -->
      <template v-if="activeTab === 'policies'">
        <div class="space-y-4">
          <div class="flex items-center justify-between">
            <p class="text-sm text-slate-400">{{ t('admin.policies.description') }}</p>
            <button @click="showNewPolicy = true" class="px-3 py-1.5 text-xs font-medium bg-blue-600 hover:bg-blue-500 text-white rounded-lg transition-colors">
              {{ t('admin.policies.new') }}
            </button>
          </div>

          <!-- Policy list -->
          <div v-if="policies.length === 0" class="bg-slate-900 border border-slate-800 rounded-xl p-8 text-center text-sm text-slate-600">
            {{ t('admin.policies.empty') }}
          </div>
          <div v-else class="space-y-3">
            <div v-for="p in policies" :key="p.id" class="bg-slate-900 border border-slate-800 rounded-xl p-4">
              <div class="flex items-start justify-between">
                <div>
                  <div class="text-sm font-medium text-slate-200">{{ p.name }}</div>
                  <div class="text-xs text-slate-500 font-mono mt-0.5">{{ p.path_pattern }}</div>
                  <div class="flex items-center gap-3 mt-2 text-xs text-slate-400">
                    <span>{{ t('admin.policies.approvals_required', p.min_approvals) }}</span>
                    <span v-if="p.required_roles?.length">{{ t('admin.policies.roles', { roles: p.required_roles.join(', ') }) }}</span>
                    <span v-if="p.required_users?.length">{{ t('admin.policies.users', { users: p.required_users.join(', ') }) }}</span>
                    <span v-if="p.require_human" class="text-amber-400">{{ t('admin.policies.human_required') }}</span>
                    <span v-if="p.auto_merge" class="text-emerald-400">{{ t('admin.policies.auto_merge') }}</span>
                  </div>
                </div>
                <button @click="deletePolicy(p.id)" class="text-slate-600 hover:text-red-400 p-1 transition-colors">
                  <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" />
                  </svg>
                </button>
              </div>
            </div>
          </div>

          <!-- New policy form -->
          <div v-if="showNewPolicy" class="bg-slate-900 border border-blue-500/30 rounded-xl p-5 space-y-4">
            <h3 class="text-sm font-semibold text-slate-200">{{ t('admin.policies.form_title') }}</h3>
            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class="block text-xs text-slate-500 mb-1">{{ t('admin.policies.name') }}</label>
                <input v-model="newPolicy.name" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500" :placeholder="t('admin.policies.name_placeholder')" />
              </div>
              <div>
                <label class="block text-xs text-slate-500 mb-1">{{ t('admin.policies.path_pattern') }}</label>
                <input v-model="newPolicy.path_pattern" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white font-mono focus:outline-none focus:border-blue-500" :placeholder="t('admin.policies.path_pattern_placeholder')" />
              </div>
              <div>
                <label class="block text-xs text-slate-500 mb-1">{{ t('admin.policies.min_approvals') }}</label>
                <input v-model.number="newPolicy.min_approvals" type="number" min="1" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500" />
              </div>
              <div>
                <label class="block text-xs text-slate-500 mb-1">{{ t('admin.policies.required_roles') }}</label>
                <input v-model="newPolicy.required_roles_str" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500" :placeholder="t('admin.policies.required_roles_placeholder')" />
              </div>
              <div class="col-span-2">
                <label class="block text-xs text-slate-500 mb-1">{{ t('admin.policies.required_users') }}</label>
                <input v-model="newPolicy.required_users_str" class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500" :placeholder="t('admin.policies.required_users_placeholder')" />
              </div>
              <div>
                <label class="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" v-model="newPolicy.require_human" class="rounded bg-slate-800 border-slate-700 text-blue-500 focus:ring-blue-500" />
                  <span class="text-xs text-slate-400">{{ t('admin.policies.require_human') }}</span>
                </label>
              </div>
              <div>
                <label class="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" v-model="newPolicy.auto_merge" class="rounded bg-slate-800 border-slate-700 text-emerald-500 focus:ring-emerald-500" />
                  <span class="text-xs text-slate-400">{{ t('admin.policies.auto_merge_label') }}</span>
                </label>
              </div>
            </div>
            <div v-if="policyError" class="text-xs text-red-400">{{ policyError }}</div>
            <div class="flex gap-2">
              <button @click="savePolicy" :disabled="policySaving" class="px-4 py-2 text-sm font-medium bg-blue-600 hover:bg-blue-500 text-white rounded-lg disabled:opacity-50">
                {{ policySaving ? t('admin.policies.creating') : t('admin.policies.create') }}
              </button>
              <button @click="showNewPolicy = false" class="px-4 py-2 text-sm text-slate-400 hover:text-white">{{ t('common.action.cancel') }}</button>
            </div>
          </div>
        </div>
      </template>

      <!-- ===================== SETTINGS TAB ===================== -->
      <template v-if="activeTab === 'settings'">
        <div class="space-y-6">
          <div v-if="Object.keys(settingsByCategory).length === 0" class="bg-slate-900 border border-slate-800 rounded-xl px-5 py-12 text-center text-sm text-slate-600">
            {{ t('admin.settings.empty') }}
          </div>
          <div v-for="(group, cat) in settingsByCategory" :key="cat" class="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden">
            <div class="px-5 py-3 border-b border-slate-800">
              <h3 class="text-xs font-semibold text-slate-400 uppercase tracking-wider">{{ settingCategoryLabel(cat) }}</h3>
            </div>
            <div class="divide-y divide-slate-800">
              <div v-for="s in group" :key="s.key" class="px-5 py-4">
                <div class="flex items-start justify-between gap-4 mb-3">
                  <div class="flex-1 min-w-0">
                    <label class="block text-sm font-medium text-slate-200">{{ humanSettingLabel(s.key) }}</label>
                    <div v-if="s.description" class="text-xs text-slate-500 mt-0.5">{{ s.description }}</div>
                  </div>
                  <button @click="saveSetting(s)" :disabled="s._saving"
                    class="px-3 py-1.5 bg-blue-600 hover:bg-blue-500 text-white text-xs font-medium rounded-lg transition-colors disabled:opacity-50 whitespace-nowrap flex-shrink-0">
                    {{ s._saving ? t('common.state.saving') : t('common.action.save') }}
                  </button>
                </div>
                <!-- Boolean toggle -->
                <label v-if="settingType(s) === 'boolean'" class="relative inline-flex items-center cursor-pointer">
                  <input type="checkbox" :checked="s.value === 'true'" @change="s.value = $event.target.checked ? 'true' : 'false'" class="sr-only peer" />
                  <div class="w-9 h-5 bg-slate-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-600"></div>
                  <!-- i18n-ignore — 'true' is the stored setting value, not copy -->
                  <span class="ml-3 text-sm text-slate-400">{{ s.value === 'true' ? t('admin.settings.enabled') : t('admin.settings.disabled') }}</span>
                </label>
                <!-- Locale select. A free-text tag here is a trap: the server 400s an
                     unrecognised one, and the admin has no way to know what it accepts. -->
                <select v-else-if="settingType(s) === 'locale'" v-model="s.value"
                  class="w-64 bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500">
                  <!-- Empty is a real, server-accepted value: no org default. -->
                  <option value="">{{ t('admin.settings.locale_none') }}</option>
                  <option v-for="l in localeOptions" :key="l.tag" :value="l.tag">{{ l.name }}</option>
                  <!-- A stored tag this build cannot render still has to be shown as the
                       current value; a select with no matching option would silently
                       display something other than what is saved. -->
                  <option v-if="s.value && !localeOptions.some(l => l.tag === s.value)" :value="s.value">{{ s.value }}</option>
                </select>
                <!-- Number input -->
                <input v-else-if="settingType(s) === 'number'" v-model="s.value" type="number" min="0"
                  class="w-32 bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500" />
                <!-- Color picker -->
                <div v-else-if="settingType(s) === 'color'" class="flex items-center gap-3">
                  <input v-model="s.value" type="color" class="w-10 h-10 rounded-lg border border-slate-700 bg-slate-800 cursor-pointer p-0.5" />
                  <input v-model="s.value" type="text"
                    class="flex-1 max-w-[200px] bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500 font-mono"
                    placeholder="#3b82f6" />
                </div>
                <!-- Secret (with reveal toggle) -->
                <div v-else-if="settingType(s) === 'secret'" class="flex items-center gap-2 w-full max-w-xl">
                  <div class="relative flex-1">
                    <input v-model="s.value" :type="s._reveal ? 'text' : 'password'"
                      class="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 pr-10 text-sm text-white focus:outline-none focus:border-blue-500 font-mono"
                      :placeholder="s.value ? '' : t('admin.settings.secret_unset')" />
                    <button @click="s._reveal = !s._reveal" type="button"
                      :title="s._reveal ? t('admin.settings.secret_hide') : t('admin.settings.secret_reveal')"
                      class="absolute right-2 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-200 p-1">
                      <svg v-if="!s._reveal" class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                        <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                        <circle cx="12" cy="12" r="3" />
                      </svg>
                      <svg v-else class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                        <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" />
                        <line x1="1" y1="1" x2="23" y2="23" />
                      </svg>
                    </button>
                  </div>
                </div>
                <!-- Text -->
                <input v-else v-model="s.value" type="text"
                  class="w-full max-w-xl bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500" />
              </div>

              <!-- Risk categories: bespoke editor (hidden from the generic list) -->
              <div v-if="cat === 'risk'" class="px-5 py-4">
                <div class="flex items-start justify-between gap-4 mb-3">
                  <div class="flex-1 min-w-0">
                    <label class="block text-sm font-medium text-slate-200">{{ t('admin.risk_categories.title') }}</label>
                    <div class="text-xs text-slate-500 mt-0.5">{{ t('admin.risk_categories.description') }}</div>
                  </div>
                  <div class="flex items-center gap-2 flex-shrink-0">
                    <button @click="resetRiskCategories" :disabled="riskCategoriesSaving || !riskCategoriesCustomized"
                      :title="riskCategoriesCustomized ? t('admin.risk_categories.reset_hint') : t('admin.risk_categories.reset_hint_disabled')"
                      class="px-3 py-1.5 bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-300 text-xs font-medium rounded-lg transition-colors disabled:opacity-40 whitespace-nowrap">
                      {{ t('admin.risk_categories.reset') }}
                    </button>
                    <button @click="saveRiskCategories" :disabled="riskCategoriesSaving"
                      class="px-3 py-1.5 bg-blue-600 hover:bg-blue-500 text-white text-xs font-medium rounded-lg transition-colors disabled:opacity-50 whitespace-nowrap">
                      {{ riskCategoriesSaving ? t('common.state.saving') : t('common.action.save') }}
                    </button>
                  </div>
                </div>

                <div class="w-full max-w-xl space-y-2">
                  <div v-if="!riskCategoriesLoaded" class="text-xs text-slate-600">{{ t('common.state.loading') }}</div>
                  <div v-else-if="riskCategories.length === 0" class="text-xs text-slate-600">
                    {{ t('admin.risk_categories.empty') }}
                  </div>
                  <div v-for="(c, i) in riskCategories" :key="c.key"
                    class="flex items-center gap-2 bg-slate-800 border border-slate-700 rounded-lg px-3 py-2">
                    <div class="flex flex-col">
                      <button @click="moveRiskCategory(i, -1)" :disabled="i === 0" :title="t('admin.risk_categories.move_up')"
                        class="text-slate-500 hover:text-slate-200 disabled:opacity-30 disabled:hover:text-slate-500 leading-none text-[10px]">▲</button>
                      <button @click="moveRiskCategory(i, 1)" :disabled="i === riskCategories.length - 1" :title="t('admin.risk_categories.move_down')"
                        class="text-slate-500 hover:text-slate-200 disabled:opacity-30 disabled:hover:text-slate-500 leading-none text-[10px]">▼</button>
                    </div>
                    <input v-model="c.label" type="text" maxlength="100" :placeholder="t('admin.risk_categories.label_placeholder')"
                      class="flex-1 min-w-0 bg-slate-900 border border-slate-700 rounded-lg px-2.5 py-1.5 text-sm text-white focus:outline-none focus:border-blue-500" />
                    <span class="text-[11px] text-slate-500 font-mono truncate max-w-[10rem]" :title="c.key">{{ c.key }}</span>
                    <button @click="removeRiskCategory(i)" :title="t('admin.risk_categories.remove')"
                      class="text-slate-500 hover:text-red-400 transition-colors flex-shrink-0">
                      <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
                      </svg>
                    </button>
                  </div>

                  <div class="flex items-center gap-2 pt-1">
                    <input v-model="newCategoryLabel" type="text" maxlength="100"
                      :placeholder="t('admin.risk_categories.new_placeholder')" @keyup.enter="addRiskCategory"
                      class="flex-1 min-w-0 bg-slate-800 border border-slate-700 rounded-lg px-2.5 py-1.5 text-sm text-white focus:outline-none focus:border-blue-500" />
                    <button @click="addRiskCategory"
                      class="px-3 py-1.5 bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-300 text-xs font-medium rounded-lg transition-colors whitespace-nowrap">
                      {{ t('admin.risk_categories.add') }}
                    </button>
                  </div>
                  <div v-if="newCategoryLabel.trim()" class="text-[10px] text-slate-600">
                    <i18n-t keypath="admin.risk_categories.key_preview" tag="span" scope="global">
                      <template #key><span class="font-mono">{{ slugifyCategory(newCategoryLabel) || '—' }}</span></template>
                    </i18n-t>
                  </div>
                  <div class="text-[10px] text-slate-600">{{ t('admin.risk_categories.footnote') }}</div>
                  <div v-if="riskCategoriesMsg" class="text-xs" :class="riskCategoriesError ? 'text-red-400' : 'text-emerald-400'">
                    {{ riskCategoriesMsg }}
                  </div>
                </div>
              </div>
            </div>
          </div>
          <div v-if="settingsMsg" class="text-xs" :class="settingsError ? 'text-red-400' : 'text-emerald-400'">{{ settingsMsg }}</div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup>
import { useConfirm } from '../composables/useConfirm'
import { slugifyCategory } from '../riskCategories'
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import api from '../api.js'
import { useSession } from '../composables/useSession'
import { useCurrentOrg } from '../composables/useCurrentOrg.js'
import { useLocale } from '../composables/useLocale'
import { formatDate } from '../composables/useFormat.js'
import { enumLabel } from '../composables/useEnumLabel.js'
import { renderApiError } from '../composables/useApiError.js'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const { orgSlug, orgPath } = useCurrentOrg()

const pageLoading = ref(true)

// --- Approval Policies ---
const policies = ref([])
const showNewPolicy = ref(false)
const newPolicy = ref({ name: '', path_pattern: '', min_approvals: 2, required_roles_str: '', required_users_str: '', require_human: true, auto_merge: false })
const policySaving = ref(false)
const policyError = ref('')

async function loadPolicies() {
  try {
    const data = await api.listPolicies()
    policies.value = Array.isArray(data?.data || data) ? (data?.data || data) : []
  } catch { policies.value = [] }
}

async function savePolicy() {
  if (!newPolicy.value.name || !newPolicy.value.path_pattern) { policyError.value = t('admin.policies.error_name_required'); return }
  policySaving.value = true
  policyError.value = ''
  try {
    await api.createPolicy({
      name: newPolicy.value.name,
      path_pattern: newPolicy.value.path_pattern,
      min_approvals: newPolicy.value.min_approvals || 1,
      required_roles: newPolicy.value.required_roles_str ? newPolicy.value.required_roles_str.split(',').map(s => s.trim()).filter(Boolean) : [],
      required_users: newPolicy.value.required_users_str ? newPolicy.value.required_users_str.split(',').map(s => s.trim()).filter(Boolean) : [],
      require_human: newPolicy.value.require_human,
      auto_merge: newPolicy.value.auto_merge,
    })
    showNewPolicy.value = false
    newPolicy.value = { name: '', path_pattern: '', min_approvals: 2, required_roles_str: '', required_users_str: '', require_human: true, auto_merge: false }
    await loadPolicies()
  } catch (e) {
    policyError.value = renderApiError(e) || t('admin.policies.error_create')
  } finally {
    policySaving.value = false
  }
}

async function deletePolicy(id) {
  const { ask } = useConfirm()
  if (!await ask(t('admin.policies.delete_confirm'))) return
  try {
    await api.deletePolicy(id)
    await loadPolicies()
  } catch (e) {
    policyError.value = renderApiError(e)
  }
}
// The three role selects use the same catalogue but keep their existing
// orders, which differ deliberately (the member table leads with admin, the
// invite form with contributor, and OIDC offers no admin default at all).
const roleOptionsFor = (values) =>
  computed(() => values.map((value) => ({ value, label: enumLabel('role', value) })))
const roleOptions = roleOptionsFor(['admin', 'manager', 'contributor', 'reader'])
const inviteRoleOptions = roleOptionsFor(['contributor', 'reader', 'manager', 'admin'])
const oidcRoleOptions = roleOptionsFor(['reader', 'contributor', 'manager'])

// The audit table rendered the stored permission string raw.
const permissionLabel = (perm) => enumLabel('permissions', perm || 'read-write')

const activeTab = ref('members')

// Message keys, not labels: a module-scope array of literals would freeze the
// tab bar in whichever locale was active when this module first evaluated. Each
// key is written out in full, since a key built by concatenation is unreachable
// to keyset extraction.
const TABS = [
  { key: 'members', label: 'admin.tab.members' },
  { key: 'invite', label: 'admin.tab.invite' },
  { key: 'api-keys', label: 'admin.tab.api_keys' },
  { key: 'authentication', label: 'admin.tab.authentication' },
  { key: 'branding', label: 'admin.tab.branding' },
  { key: 'policies', label: 'admin.tab.policies' },
  { key: 'settings', label: 'admin.tab.settings' },
]

const tabs = computed(() => TABS.map((tab) => ({ ...tab, label: t(tab.label) })))

// Members
const members = ref([])
const membersMsg = ref('')
const membersError = ref(false)

// Invite
const inviteEmail = ref('')
const inviteName = ref('')
const inviteRole = ref('reader')
const inviting = ref(false)
const inviteMsg = ref('')
const inviteError = ref(false)

// API Keys (read-only audit view)
const apiKeys = ref([])

// Authentication
const oidcProviders = ref([])
const newProvider = ref({})
// providerType drives which extra inputs are shown:
//   'microsoft' → tenantId input, 'google' → none, 'okta' → oktaDomain input,
//   '' → custom (raw discovery URL), null → form hidden (no preset chosen yet).
const providerType = ref(null)
const tenantId = ref('')
const oktaDomain = ref('')
const addingProvider = ref(false)
const newProviderMsg = ref('')
const newProviderError = ref(false)

// Settings
// Renamed on destructure: `options` is too generic for a 1000-line view. Already
// seeded by useSession's loadBranding() -> applyConfigLocales on any org page, and
// it is a ref, so a late /config fill-in reaches the select on its own.
const { options: localeOptions } = useLocale()

const settings = ref([])
const settingsMsg = ref('')
const settingsError = ref(false)

// Branding
const branding = ref({
  branding_name: '',
  branding_color: '#3b82f6',
  branding_footer: '',
  show_powered_by: '',
  terms_url: '',
  privacy_url: '',
})
const brandingSaving = ref(false)
const brandingMsg = ref('')
const brandingError = ref(false)
const { loadBranding } = useSession()
const uploadMsg = ref({})
const uploadErr = ref({})
const logoPreviewUrl = ref('')
const faviconPreviewUrl = ref('')

async function uploadBrandingFile(event, type) {
  const file = event.target.files?.[0]
  if (!file) return
  uploadMsg.value = { ...uploadMsg.value, [type]: '' }
  uploadErr.value = { ...uploadErr.value, [type]: false }
  if (file.size > 2 * 1024 * 1024) {
    uploadMsg.value = { ...uploadMsg.value, [type]: t('admin.branding.error_too_large') }
    uploadErr.value = { ...uploadErr.value, [type]: true }
    return
  }
  try {
    await api.uploadBranding(file, type)
    uploadMsg.value = { ...uploadMsg.value, [type]: type === 'favicon' ? t('admin.branding.favicon_uploaded') : t('admin.branding.logo_uploaded') }
    uploadErr.value = { ...uploadErr.value, [type]: false }
    const stamp = Date.now()
    if (type === 'logo') logoPreviewUrl.value = '/branding/logo?t=' + stamp
    if (type === 'favicon') faviconPreviewUrl.value = '/branding/favicon.ico?t=' + stamp
    await loadBranding()
  } catch (e) {
    uploadMsg.value = { ...uploadMsg.value, [type]: renderApiError(e) || t('admin.branding.error_upload') }
    uploadErr.value = { ...uploadErr.value, [type]: true }
  }
  event.target.value = ''
}

const showPoweredByToggle = computed({
  get: () => branding.value.show_powered_by !== 'false',
  set: (v) => { branding.value.show_powered_by = v ? 'true' : 'false' },
})

function switchTab(key) {
  activeTab.value = key
  router.replace(orgPath(`/admin/${key}`))
}

function providerIconClass(name) {
  if (name === 'microsoft' || name === 'azure') return 'bg-[#2F2F2F] text-white'
  if (name === 'google') return 'bg-white text-gray-700'
  return 'bg-slate-700 text-slate-200'
}

function resetNewProvider() {
  newProvider.value = {}
  providerType.value = null
  tenantId.value = ''
  oktaDomain.value = ''
  newProviderMsg.value = ''
}

function applyPreset(type) {
  const base = { enabled: true, auto_add_members: false, default_role: 'reader' }
  newProviderMsg.value = ''
  newProviderError.value = false
  tenantId.value = ''
  oktaDomain.value = ''
  if (type === 'microsoft') {
    providerType.value = 'microsoft'
    newProvider.value = { ...base, provider_name: 'microsoft', display_name: 'Microsoft 365', discovery_url: '', client_id: '', client_secret: '' }
  } else if (type === 'google') {
    providerType.value = 'google'
    newProvider.value = { ...base, provider_name: 'google', display_name: 'Google Workspace', discovery_url: 'https://accounts.google.com/.well-known/openid-configuration', client_id: '', client_secret: '' }
  } else if (type === 'okta') {
    providerType.value = 'okta'
    newProvider.value = { ...base, provider_name: 'okta', display_name: 'Okta', discovery_url: '', client_id: '', client_secret: '' }
  } else {
    // custom / generic
    providerType.value = ''
    newProvider.value = { ...base, provider_name: '', display_name: '', discovery_url: '', client_id: '', client_secret: '' }
  }
}

// Build the discovery URL from provider-type specific inputs. Returns
// { url, error } — error is a user-facing message when required input is missing.
function buildDiscoveryURL() {
  // The Go go-oidc library takes the ISSUER URL (not the discovery URL) and
  // appends `/.well-known/openid-configuration` itself. Store the issuer; the
  // column is named discovery_url for legacy reasons but holds the issuer.
  if (providerType.value === 'microsoft') {
    const tenant = (tenantId.value || '').trim()
    if (!tenant) return { url: '', error: t('admin.oidc.error_tenant_required') }
    return { url: `https://login.microsoftonline.com/${encodeURIComponent(tenant)}/v2.0`, error: '' }
  }
  if (providerType.value === 'google') {
    return { url: 'https://accounts.google.com', error: '' }
  }
  if (providerType.value === 'okta') {
    let d = (oktaDomain.value || '').trim()
    if (!d) return { url: '', error: t('admin.oidc.error_okta_required') }
    // Strip protocol and trailing slash if user pasted a full URL.
    d = d.replace(/^https?:\/\//i, '').replace(/\/+$/, '')
    return { url: `https://${d}`, error: '' }
  }
  // custom / empty — accept either the issuer or the discovery URL and
  // normalise: trim a trailing `/.well-known/openid-configuration` if present.
  let u = (newProvider.value.discovery_url || '').trim()
  if (!u) return { url: '', error: t('admin.oidc.error_issuer_required') }
  u = u.replace(/\/+$/, '')
  u = u.replace(/\/\.well-known\/openid-configuration$/, '')
  return { url: u, error: '' }
}

// ---- API calls ----

async function loadMembers() {
  try {
    const data = await api.fetchJSON('/api/v1/admin/members')
    members.value = Array.isArray(data) ? data : (data?.data || [])
  } catch (e) {
    membersMsg.value = renderApiError(e)
    membersError.value = true
  }
}

async function updateMemberRole(member, newRole) {
  membersMsg.value = ''
  try {
    await api.putJSON(`/api/v1/admin/members/${member.id || member.email}/role`, { role: newRole })
    member.role = newRole
    membersMsg.value = t('admin.members.role_updated', { email: member.email, role: enumLabel('role', newRole) })
    membersError.value = false
  } catch (e) {
    membersMsg.value = renderApiError(e)
    membersError.value = true
  }
}

async function removeMember(member) {
  if (!await useConfirm().ask(t('admin.members.remove_confirm', { email: member.email }))) return
  membersMsg.value = ''
  try {
    await api.deleteJSON(`/api/v1/admin/members/${member.id || member.email}`)
    members.value = members.value.filter(m => m.email !== member.email)
    membersMsg.value = t('admin.members.removed', { email: member.email })
    membersError.value = false
  } catch (e) {
    membersMsg.value = renderApiError(e)
    membersError.value = true
  }
}

async function sendInvite() {
  inviting.value = true
  inviteMsg.value = ''
  try {
    await api.postJSON('/api/v1/auth/invite', { email: inviteEmail.value, name: inviteName.value, role: inviteRole.value })
    inviteMsg.value = t('admin.invite.sent', { email: inviteEmail.value })
    inviteError.value = false
    inviteEmail.value = ''
    inviteName.value = ''
    await loadMembers()
  } catch (e) {
    inviteMsg.value = renderApiError(e)
    inviteError.value = true
  } finally {
    inviting.value = false
  }
}

async function resendInvite(member) {
  membersMsg.value = ''
  membersError.value = false
  try {
    await api.postJSON('/api/v1/auth/resend-invite', { email: member.email })
    membersMsg.value = t('admin.members.invite_resent', { email: member.email })
    membersError.value = false
  } catch (e) {
    membersMsg.value = renderApiError(e)
    membersError.value = true
  }
}

async function loadAPIKeys() {
  try {
    const data = await api.fetchJSON('/api/v1/admin/api-keys')
    apiKeys.value = Array.isArray(data) ? data : (data?.data || [])
  } catch { /* ignore */ }
}

async function loadOIDCProviders() {
  try {
    const data = await api.fetchJSON('/api/v1/admin/oidc')
    oidcProviders.value = (Array.isArray(data) ? data : (data?.data || [])).map(p => ({ ...p, _saving: false, _testing: false, _msg: '', _error: false }))
  } catch { /* ignore */ }
}

async function saveProvider(provider) {
  provider._saving = true
  provider._msg = ''
  try {
    await api.putJSON(`/api/v1/admin/oidc/${provider.id}`, {
      provider_name: provider.provider_name,
      display_name: provider.display_name,
      client_id: provider.client_id,
      client_secret: provider.client_secret,
      discovery_url: provider.discovery_url,
      enabled: provider.enabled,
      auto_add_members: provider.auto_add_members,
      default_role: provider.default_role,
    })
    provider._msg = t('admin.oidc.saved')
    provider._error = false
  } catch (e) {
    provider._msg = renderApiError(e)
    provider._error = true
  } finally {
    provider._saving = false
  }
}

async function testProvider(provider) {
  provider._testing = true
  provider._msg = ''
  try {
    const result = await api.postJSON(`/api/v1/admin/oidc/${provider.id}/test`, {})
    provider._msg = result.message || t('admin.oidc.test_ok')
    provider._error = false
  } catch (e) {
    provider._msg = renderApiError(e) || t('admin.oidc.error_test')
    provider._error = true
  } finally {
    provider._testing = false
  }
}

async function deleteProvider(provider) {
  if (!await useConfirm().ask(t('admin.oidc.delete_confirm', { name: provider.display_name || provider.name }))) return
  try {
    await api.deleteJSON(`/api/v1/admin/oidc/${provider.id}`)
    oidcProviders.value = oidcProviders.value.filter(p => p.id !== provider.id)
  } catch (e) {
    provider._msg = renderApiError(e)
    provider._error = true
  }
}

async function addProvider() {
  newProviderMsg.value = ''
  newProviderError.value = false
  const { url, error } = buildDiscoveryURL()
  if (error) {
    newProviderMsg.value = error
    newProviderError.value = true
    return
  }
  if (!newProvider.value.provider_name || !newProvider.value.provider_name.trim()) {
    newProviderMsg.value = t('admin.oidc.error_provider_name_required')
    newProviderError.value = true
    return
  }
  if (!newProvider.value.client_id || !newProvider.value.client_id.trim()) {
    newProviderMsg.value = t('admin.oidc.error_client_id_required')
    newProviderError.value = true
    return
  }
  addingProvider.value = true
  try {
    await api.postJSON('/api/v1/admin/oidc', { ...newProvider.value, discovery_url: url })
    newProviderMsg.value = t('admin.oidc.added')
    newProviderError.value = false
    resetNewProvider()
    await loadOIDCProviders()
  } catch (e) {
    newProviderMsg.value = renderApiError(e)
    newProviderError.value = true
  } finally {
    addingProvider.value = false
  }
}

async function loadSettings() {
  try {
    const data = await api.fetchJSON('/api/v1/admin/settings')
    settings.value = (Array.isArray(data) ? data : (data?.data || [])).map(s => ({ ...s, _saving: false, _reveal: false }))
    syncRiskCategoriesCustomized()
  } catch { /* ignore */ }
}

// Branding-tab handles these — hide from generic Settings tab to avoid duplication.
const BRANDING_SETTING_KEYS = new Set([
  'branding_name', 'branding_color', 'branding_footer',
  'show_powered_by', 'terms_url', 'privacy_url',
])

// Edited by dedicated UI (the Risk Categories editor in the Risk group) — hide
// from the generic list so it does not render as a raw JSON text field.
const HIDDEN_SETTING_KEYS = new Set(['risk_categories'])

// A closed set of categories the server groups settings by, each with its own
// message key. An unknown category falls back to its own name rather than
// rendering blank — the same shape as enumLabel's de-slug fallback.
const SETTING_CATEGORY_KEYS = {
  localization: 'admin.settings.category.localization',
  notifications: 'admin.settings.category.notifications',
  review_cycles: 'admin.settings.category.review_cycles',
  risk: 'admin.settings.category.risk',
  ai: 'admin.settings.category.ai',
  other: 'admin.settings.category.other',
}

function settingCategoryLabel(cat) {
  const key = SETTING_CATEGORY_KEYS[cat]
  return key ? t(key) : cat
}

const settingsByCategory = computed(() => {
  const groups = {}
  for (const s of settings.value) {
    if (BRANDING_SETTING_KEYS.has(s.key)) continue
    if (HIDDEN_SETTING_KEYS.has(s.key)) continue
    const cat = s.category || 'other'
    if (!groups[cat]) groups[cat] = []
    groups[cat].push(s)
  }
  return groups
})

// Infer widget type from setting metadata. Order matters: locale > color > secret >
// boolean > number > text. The locale check is by key rather than by value shape —
// the set of valid tags is known, so this is a choice, not free text.
function settingType(s) {
  if (s.key === 'default_locale') return 'locale'
  if (s.key.endsWith('_color')) return 'color'
  if (s.sensitive) return 'secret'
  const v = s.value || s.default_value || ''
  if (v === 'true' || v === 'false') return 'boolean'
  if (/^-?\d+$/.test(v)) return 'number'
  return 'text'
}

function humanSettingLabel(key) {
  return key.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
}

async function saveSetting(s) {
  s._saving = true
  settingsMsg.value = ''
  try {
    await api.putJSON('/api/v1/admin/settings', { key: s.key, value: s.value })
    settingsMsg.value = t('admin.settings.saved', { key: s.key })
    settingsError.value = false
  } catch (e) {
    settingsMsg.value = renderApiError(e)
    settingsError.value = true
  } finally {
    s._saving = false
  }
}

// ---------- Risk categories editor ----------
// Categories are org-authored data: labels are stored verbatim and are never
// routed through a translation layer. Keys are slugs, frozen at creation, because
// risks store the key — renaming a label must never need a data migration.
const RISK_CATEGORY_KEY_RE = /^[a-z0-9]+(_[a-z0-9]+)*$/
const RISK_CATEGORY_MAX = 50

const riskCategories = ref([])
const riskCategoriesLoaded = ref(false)
const riskCategoriesSaving = ref(false)
const riskCategoriesMsg = ref('')
const riskCategoriesError = ref(false)
const newCategoryLabel = ref('')

// Whether the org has a stored list of its own, as opposed to following the
// built-in defaults. Derived from the RAW setting value (empty until an admin
// saves one) — the effective list from GET /risks/categories cannot tell the
// two apart. Gates the reset action, which is the only way back to defaults.
const riskCategoriesCustomized = ref(false)

function syncRiskCategoriesCustomized() {
  const raw = settings.value.find(s => s.key === 'risk_categories')
  riskCategoriesCustomized.value = !!(raw?.value || '').trim()
}

async function loadRiskCategories() {
  // Reads the *effective* list (custom, or the server defaults when unconfigured),
  // not the raw setting value — which is NULL until an admin saves one.
  try {
    const data = await api.fetchJSON('/api/v1/risks/categories')
    riskCategories.value = (Array.isArray(data) ? data : []).map(c => ({ key: c.key, label: c.label }))
  } catch {
    riskCategories.value = []
  } finally {
    riskCategoriesLoaded.value = true
  }
}


function setCategoryError(msg) {
  riskCategoriesMsg.value = msg
  riskCategoriesError.value = true
}

function addRiskCategory() {
  const label = newCategoryLabel.value.trim()
  riskCategoriesMsg.value = ''
  if (!label) return setCategoryError(t('admin.risk_categories.error_label_required'))
  if (label.length > 100) return setCategoryError(t('admin.risk_categories.error_label_too_long'))
  if (riskCategories.value.length >= RISK_CATEGORY_MAX) {
    return setCategoryError(t('admin.risk_categories.error_too_many', { max: RISK_CATEGORY_MAX }))
  }
  const key = slugifyCategory(label)
  if (!key || !RISK_CATEGORY_KEY_RE.test(key)) {
    return setCategoryError(t('admin.risk_categories.error_key_undeliverable'))
  }
  if (riskCategories.value.some(c => c.key.toLowerCase() === key)) {
    return setCategoryError(t('admin.risk_categories.error_duplicate_key', { key }))
  }
  riskCategories.value.push({ key, label })
  newCategoryLabel.value = ''
  riskCategoriesError.value = false
}

function moveRiskCategory(i, delta) {
  const j = i + delta
  if (j < 0 || j >= riskCategories.value.length) return
  const list = riskCategories.value
  ;[list[i], list[j]] = [list[j], list[i]]
}

async function removeRiskCategory(i) {
  const cat = riskCategories.value[i]
  if (!cat) return
  if (riskCategories.value.length <= 1) {
    return setCategoryError(t('admin.risk_categories.error_min_one'))
  }
  const ok = await useConfirm().ask(
    t('admin.risk_categories.remove_confirm', { label: cat.label }),
    { confirm: t('admin.risk_categories.remove_confirm_action'), variant: 'danger' },
  )
  if (!ok) return
  riskCategories.value.splice(i, 1)
  riskCategoriesMsg.value = ''
  riskCategoriesError.value = false
}

async function saveRiskCategories() {
  riskCategoriesMsg.value = ''
  const list = riskCategories.value.map(c => ({ key: c.key, label: (c.label || '').trim() }))
  if (list.length < 1 || list.length > RISK_CATEGORY_MAX) {
    return setCategoryError(t('admin.risk_categories.error_count_range', { max: RISK_CATEGORY_MAX }))
  }
  const seen = new Set()
  for (const c of list) {
    if (!c.label) return setCategoryError(t('admin.risk_categories.error_label_empty'))
    if (c.label.length > 100) return setCategoryError(t('admin.risk_categories.error_label_overlong', { label: c.label.slice(0, 20) }))
    if (c.key.length > 64 || !RISK_CATEGORY_KEY_RE.test(c.key)) {
      return setCategoryError(t('admin.risk_categories.error_invalid_key', { key: c.key }))
    }
    const lower = c.key.toLowerCase()
    if (seen.has(lower)) return setCategoryError(t('admin.risk_categories.error_duplicate', { key: c.key }))
    seen.add(lower)
  }
  riskCategoriesSaving.value = true
  try {
    await api.putJSON('/api/v1/admin/settings', { key: 'risk_categories', value: JSON.stringify(list) })
    riskCategories.value = list
    riskCategoriesCustomized.value = true
    riskCategoriesMsg.value = t('admin.risk_categories.saved')
    riskCategoriesError.value = false
  } catch (e) {
    setCategoryError(renderApiError(e))
  } finally {
    riskCategoriesSaving.value = false
  }
}

// Clears the org's stored list by writing an empty value, which the API treats as
// "follow the built-in defaults". Without this an org that has ever saved a custom
// list could never get back: Save rejects an empty list, removal refuses to delete
// the last entry, and the raw setting is hidden from the generic settings list.
async function resetRiskCategories() {
  if (!await useConfirm().ask(
    t('admin.risk_categories.reset_confirm'),
    { confirm: t('admin.risk_categories.reset_confirm_action'), variant: 'danger' },
  )) return
  riskCategoriesMsg.value = ''
  riskCategoriesSaving.value = true
  try {
    await api.putJSON('/api/v1/admin/settings', { key: 'risk_categories', value: '' })
    riskCategoriesCustomized.value = false
    await loadRiskCategories()
    riskCategoriesMsg.value = t('admin.risk_categories.reset_done')
    riskCategoriesError.value = false
  } catch (e) {
    setCategoryError(renderApiError(e))
  } finally {
    riskCategoriesSaving.value = false
  }
}

function loadBrandingFromSettings() {
  // Extract branding keys from the already-loaded settings list
  for (const s of settings.value) {
    if (s.key in branding.value) {
      branding.value[s.key] = s.value || branding.value[s.key]
    }
  }
  // Check if assets already exist
  logoPreviewUrl.value = '/branding/logo?t=' + Date.now()
  faviconPreviewUrl.value = '/branding/favicon.ico?t=' + Date.now()
}

async function saveBranding() {
  brandingSaving.value = true
  brandingMsg.value = ''
  try {
    const keys = ['branding_name', 'branding_color', 'branding_footer', 'show_powered_by', 'terms_url', 'privacy_url']
    for (const key of keys) {
      await api.putJSON('/api/v1/admin/settings', { key, value: branding.value[key] })
    }
    // Apply color immediately
    if (branding.value.branding_color) {
      document.documentElement.style.setProperty('--brand-color', branding.value.branding_color)
    }
    brandingMsg.value = t('admin.branding.saved')
    brandingError.value = false
  } catch (e) {
    brandingMsg.value = renderApiError(e)
    brandingError.value = true
  } finally {
    brandingSaving.value = false
  }
}

// ---- Init ----

onMounted(async () => {
  // Set tab from route param
  const tab = route.params.tab
  if (tab && TABS.some((entry) => entry.key === tab)) {
    activeTab.value = tab
  }

  try {
    await Promise.all([loadMembers(), loadAPIKeys(), loadOIDCProviders(), loadSettings(), loadPolicies(), loadRiskCategories()])
    loadBrandingFromSettings()
  } finally {
    pageLoading.value = false
  }
})

watch(() => route.params.tab, (tab) => {
  if (tab && TABS.some((entry) => entry.key === tab)) {
    activeTab.value = tab
  }
})
</script>
