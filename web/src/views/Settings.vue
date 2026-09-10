<template>
  <div class="max-w-2xl mx-auto px-6 py-8 space-y-8">
    <h1 class="text-xl font-bold text-white">{{ t('settings.title') }}</h1>

    <!-- Profile -->
    <section class="bg-slate-900 border border-slate-800 rounded-xl p-6">
      <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider mb-4">{{ t('settings.profile.title') }}</h2>
      <div class="space-y-4">
        <div>
          <label class="block text-xs text-slate-500 mb-1">{{ t('settings.profile.email') }}</label>
          <div class="text-sm text-slate-300">{{ user?.email }}</div>
        </div>
        <div>
          <label class="block text-xs text-slate-500 mb-1">{{ t('settings.profile.name') }}</label>
          <div class="flex gap-2 items-center">
            <input v-model="profileName" type="text"
              class="flex-1 px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500" />
            <button @click="saveName" :disabled="savingName"
              class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm rounded-lg transition-colors disabled:opacity-50">
              {{ savingName ? t('common.state.saving') : t('common.action.save') }}
            </button>
          </div>
          <div v-if="nameMsg" class="text-xs mt-1" :class="nameError ? 'text-red-400' : 'text-emerald-400'">{{ nameMsg }}</div>
        </div>
        <!-- No wrapper of our own: the picker hides itself on a single-locale
             deployment, and a wrapper here would survive it as an empty row in
             the surrounding space-y-4. The label and the error go through the
             component for the same reason — an error can only happen when the
             picker is on screen. -->
        <LocalePicker :label="$t('common.locale.label')" @error="localeMsg = $event">
          <div v-if="localeMsg" class="text-xs mt-1 text-red-400">{{ localeMsg }}</div>
        </LocalePicker>
        <div>
          <label class="block text-xs text-slate-500 mb-1">{{ t('settings.profile.role') }}</label>
          <span class="inline-block px-2 py-0.5 text-xs font-medium rounded-full"
            :class="user?.role === 'admin' ? 'bg-purple-500/20 text-purple-300' :
                     user?.role === 'manager' ? 'bg-blue-500/20 text-blue-300' :
                     'bg-slate-500/20 text-slate-400'">
            {{ roleLabel(user?.role) }}
          </span>
        </div>
      </div>
    </section>

    <!-- Change Password (only for local login users) -->
    <section v-if="user?.has_password" class="bg-slate-900 border border-slate-800 rounded-xl p-6">
      <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider mb-4">{{ t('settings.password.title') }}</h2>
      <div class="space-y-3">
        <div>
          <label class="block text-xs text-slate-500 mb-1">{{ t('settings.password.current') }}</label>
          <input v-model="currentPassword" type="password"
            class="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500" />
        </div>
        <div>
          <label class="block text-xs text-slate-500 mb-1">{{ t('settings.password.new') }}</label>
          <input v-model="newPassword" type="password"
            class="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500" />
        </div>
        <button @click="changePassword" :disabled="changingPw"
          class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm rounded-lg transition-colors disabled:opacity-50">
          {{ changingPw ? t('settings.password.changing') : t('settings.password.submit') }}
        </button>
        <div v-if="pwMsg" class="text-xs" :class="pwError ? 'text-red-400' : 'text-emerald-400'">{{ pwMsg }}</div>
      </div>
    </section>

    <!-- Change Email -->
    <section class="bg-slate-900 border border-slate-800 rounded-xl p-6">
      <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider mb-4">{{ t('settings.email.title') }}</h2>
      <div class="space-y-3">
        <div>
          <label class="block text-xs text-slate-500 mb-1">{{ t('settings.email.current') }}</label>
          <div class="text-sm text-slate-300">{{ user?.email }}</div>
        </div>

        <div v-if="user?.pending_email"
          class="text-xs text-amber-300 bg-amber-500/10 border border-amber-500/30 rounded-lg px-3 py-2">
          <div>
            <i18n-t keypath="settings.email.pending" tag="span" scope="global">
              <template #email><strong>{{ user.pending_email }}</strong></template>
            </i18n-t>
            {{ t('settings.email.pending_note') }}
          </div>
          <button @click="cancelEmailChange" :disabled="cancellingEmail"
            class="mt-2 px-3 py-1 bg-amber-500/20 hover:bg-amber-500/30 text-amber-200 rounded-md border border-amber-500/40 transition-colors disabled:opacity-50">
            {{ cancellingEmail ? t('settings.email.cancelling') : t('settings.email.cancel_pending') }}
          </button>
        </div>

        <div>
          <label class="block text-xs text-slate-500 mb-1">{{ t('settings.email.new') }}</label>
          <input v-model="newEmail" type="email" autocomplete="email"
            class="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500" />
        </div>
        <div v-if="user?.has_password">
          <label class="block text-xs text-slate-500 mb-1">{{ t('settings.password.current') }}</label>
          <input v-model="emailCurrentPassword" type="password" autocomplete="current-password"
            class="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500" />
        </div>
        <div v-if="user?.otp_enabled">
          <label class="block text-xs text-slate-500 mb-1">{{ t('settings.email.otp') }}</label>
          <input v-model="emailOtp" type="text" maxlength="6" inputmode="numeric" placeholder="000000"
            @input="emailOtp = emailOtp.replace(/\D/g, '').slice(0, 6)"
            class="w-28 bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white tracking-widest text-center focus:outline-none focus:border-blue-500" />
        </div>
        <button @click="changeEmail" :disabled="changingEmail || !newEmail"
          class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm rounded-lg transition-colors disabled:opacity-50">
          {{ changingEmail ? t('settings.email.sending') : t('settings.email.submit') }}
        </button>
        <p class="text-xs text-slate-500">{{ t('settings.email.note') }}</p>
        <div v-if="emailMsg" class="text-xs" :class="emailError ? 'text-red-400' : 'text-emerald-400'">{{ emailMsg }}</div>
      </div>
    </section>

    <!-- Two-Factor Authentication -->
    <section class="bg-slate-900 border border-slate-800 rounded-xl p-6">
      <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider mb-4">{{ t('settings.otp.title') }}</h2>

      <!-- OTP enabled -->
      <div v-if="otpEnabled" class="space-y-3">
        <div class="flex items-center gap-2">
          <span class="inline-block w-2 h-2 rounded-full bg-emerald-400"></span>
          <span class="text-sm text-emerald-400">{{ t('settings.otp.enabled') }}</span>
        </div>
        <p class="text-xs text-slate-500">{{ t('settings.otp.enabled_note') }}</p>
        <p class="text-xs text-slate-500">{{ t('settings.otp.disable_note') }}</p>
        <div class="flex items-center gap-2">
          <input v-model="disableCode" type="text" maxlength="6" inputmode="numeric" placeholder="000000"
            @input="disableCode = disableCode.replace(/\D/g, '').slice(0, 6)"
            class="w-28 bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm text-white tracking-widest text-center focus:outline-none focus:border-red-500" />
          <button @click="disableOTP" :disabled="disablingOTP || disableCode.length !== 6"
            class="px-4 py-2 bg-red-600/20 hover:bg-red-600/30 text-red-400 text-sm rounded-lg border border-red-600/30 transition-colors disabled:opacity-50">
            {{ disablingOTP ? t('settings.otp.disabling') : t('settings.otp.disable') }}
          </button>
        </div>
      </div>

      <!-- OTP setup flow -->
      <div v-else class="space-y-4">
        <div v-if="!otpSecret" class="space-y-3">
          <p class="text-sm text-slate-400">{{ t('settings.otp.setup_note') }}</p>
          <button @click="setupOTP" :disabled="settingUpOTP"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm rounded-lg transition-colors disabled:opacity-50">
            {{ settingUpOTP ? t('settings.otp.setting_up') : t('settings.otp.enable') }}
          </button>
        </div>

        <!-- Step 2: Show QR + secret + verify -->
        <div v-else class="space-y-4">
          <p class="text-sm text-slate-400">{{ t('settings.otp.scan') }}</p>

          <!-- QR Code -->
          <div class="flex justify-center">
            <div class="bg-white p-4 rounded-xl">
              <canvas ref="qrCanvas"></canvas>
            </div>
          </div>

          <!-- Secret for manual entry -->
          <div>
            <label class="block text-xs text-slate-500 mb-1">{{ t('settings.otp.manual_key') }}</label>
            <div class="flex items-center gap-2">
              <code class="flex-1 px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-amber-300 font-mono tracking-wider select-all">
                {{ otpSecret }}
              </code>
              <button @click="copySecret"
                class="px-3 py-2 bg-slate-700 hover:bg-slate-600 text-slate-300 text-sm rounded-lg transition-colors">
                {{ copied ? t('common.state.copied') : t('common.action.copy') }}
              </button>
            </div>
          </div>

          <!-- Verify code -->
          <div>
            <label class="block text-xs text-slate-500 mb-1">{{ t('settings.otp.verify_label') }}</label>
            <div class="flex gap-2 items-center">
              <input v-model="otpCode" type="text" maxlength="6" placeholder="000000"
                class="w-32 px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white font-mono tracking-widest text-center focus:outline-none focus:border-blue-500"
                @keyup.enter="verifyOTP" />
              <button @click="verifyOTP" :disabled="verifyingOTP"
                class="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-white text-sm rounded-lg transition-colors disabled:opacity-50">
                {{ verifyingOTP ? t('settings.otp.verifying') : t('settings.otp.verify') }}
              </button>
            </div>
          </div>

          <button @click="cancelOTPSetup" class="text-xs text-slate-500 hover:text-slate-400">{{ t('common.action.cancel') }}</button>
        </div>
      </div>

      <div v-if="otpMsg" class="text-xs mt-2" :class="otpError ? 'text-red-400' : 'text-emerald-400'">{{ otpMsg }}</div>
    </section>

    <!-- Personal Access Tokens -->
    <section class="bg-slate-900 border border-slate-800 rounded-xl p-6">
      <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider mb-4">{{ t('settings.tokens.title') }}</h2>
      <p class="text-sm text-slate-400 mb-4">{{ t('settings.tokens.description') }}</p>

      <!-- Create token form -->
      <div class="space-y-3 mb-6">
        <div class="flex gap-2 items-end">
          <div class="flex-1">
            <label class="block text-xs text-slate-500 mb-1">{{ t('settings.tokens.name') }}</label>
            <input v-model="tokenName" type="text" :placeholder="t('settings.tokens.name_placeholder')"
              class="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500" />
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1">{{ t('settings.tokens.permissions') }}</label>
            <select v-model="tokenPermissions"
              class="px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500">
              <option v-for="p in tokenPermissionOptions" :key="p.value" :value="p.value">{{ p.label }}</option>
            </select>
          </div>
          <button @click="createToken" :disabled="creatingToken || !tokenName"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm rounded-lg transition-colors disabled:opacity-50">
            {{ creatingToken ? t('settings.tokens.creating') : t('settings.tokens.create') }}
          </button>
        </div>
        <div v-if="newTokenValue" class="p-3 bg-emerald-950/40 border border-emerald-900/50 rounded-lg">
          <p class="text-xs text-emerald-400 mb-1">{{ t('settings.tokens.created_warning') }}</p>
          <code class="block text-sm text-emerald-300 font-mono break-all select-all">{{ newTokenValue }}</code>
        </div>
        <div v-if="tokenMsg" class="text-xs" :class="tokenError ? 'text-red-400' : 'text-emerald-400'">{{ tokenMsg }}</div>
      </div>

      <!-- Token list -->
      <div v-if="tokens.length > 0" class="space-y-2">
        <div v-for="tok in tokens" :key="tok.id"
          class="flex items-center gap-3 px-4 py-3 bg-slate-800 border border-slate-700 rounded-lg">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2">
              <span class="text-sm font-medium text-slate-200">{{ tok.name }}</span>
              <span class="text-xs px-1.5 py-0.5 rounded bg-slate-700 text-slate-400">{{ permissionLabel(tok.permissions) }}</span>
              <span v-if="tok.revoked_at" class="text-xs px-1.5 py-0.5 rounded bg-red-500/20 text-red-400">{{ t('settings.tokens.revoked_badge') }}</span>
            </div>
            <div class="text-xs text-slate-500 mt-0.5">
              {{ t('settings.tokens.created_at', { date: tok.created_at ? formatDate(tok.created_at) : t('common.state.unknown') }) }}
              <span v-if="tok.last_used_at" class="ml-2">{{ t('settings.tokens.last_used', { date: formatDate(tok.last_used_at) }) }}</span>
              <span v-else class="ml-2">{{ t('settings.tokens.never_used') }}</span>
            </div>
          </div>
          <button v-if="!tok.revoked_at" @click="revokeToken(tok)"
            class="text-xs text-red-400 hover:text-red-300 px-2 py-1 rounded hover:bg-red-500/10 transition-colors flex-shrink-0">
            {{ t('settings.tokens.revoke') }}
          </button>
        </div>
      </div>
      <div v-else class="text-sm text-slate-600">{{ t('settings.tokens.empty') }}</div>
    </section>

    <!-- Passkeys -->
    <section class="bg-slate-900 border border-slate-800 rounded-xl p-6">
      <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider mb-4">{{ t('settings.passkeys.title') }}</h2>
      <p class="text-sm text-slate-400 mb-4">{{ t('settings.passkeys.description') }}</p>

      <!-- Existing passkeys -->
      <div v-if="passkeys.length > 0" class="space-y-2 mb-4">
        <div v-for="pk in passkeys" :key="pk.id"
          class="flex items-center gap-3 px-4 py-3 bg-slate-800 border border-slate-700 rounded-lg">
          <svg class="w-5 h-5 text-slate-400 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 11c0 3.517-1.009 6.799-2.753 9.571m-3.44-2.04l.054-.09A13.916 13.916 0 008 11a4 4 0 118 0c0 1.017-.07 2.019-.203 3m-2.118 6.844A21.88 21.88 0 0015.171 17m3.839 1.132c.645-2.266.99-4.659.99-7.132A8 8 0 008 4.07M3 15.364c.64-1.319 1-2.8 1-4.364 0-1.457.39-2.823 1.07-4" />
          </svg>
          <div class="flex-1 min-w-0">
            <template v-if="pk._editing">
              <div class="flex items-center gap-2">
                <input v-model="pk._editName" type="text"
                  @keydown.enter="savePasskeyName(pk)"
                  @keydown.escape="pk._editing = false"
                  class="flex-1 bg-slate-900 border border-slate-600 rounded px-2 py-1 text-sm text-white focus:outline-none focus:border-blue-500" />
                <button @click="savePasskeyName(pk)" class="text-xs text-blue-400 hover:text-blue-300">{{ t('common.action.save') }}</button>
                <button @click="pk._editing = false" class="text-xs text-slate-500 hover:text-slate-400">{{ t('common.action.cancel') }}</button>
              </div>
            </template>
            <template v-else>
              <div class="flex items-center gap-2">
                <span class="text-sm font-medium text-slate-200">{{ pk.name || t('settings.passkeys.default_name') }}</span>
                <button @click="startPasskeyRename(pk)" :aria-label="t('settings.passkeys.rename')"
                  class="text-slate-600 hover:text-slate-400 transition-colors">
                  <svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
                  </svg>
                </button>
              </div>
              <div class="text-xs text-slate-500 mt-0.5">
                {{ t('settings.passkeys.added', { date: pk.created_at ? formatDate(pk.created_at) : t('common.state.unknown') }) }}
                <span v-if="pk.last_used_at" class="ml-2">{{ t('settings.passkeys.last_used', { date: formatDate(pk.last_used_at) }) }}</span>
              </div>
            </template>
          </div>
          <button @click="deletePasskey(pk)"
            class="text-xs text-red-400 hover:text-red-300 px-2 py-1 rounded hover:bg-red-500/10 transition-colors flex-shrink-0">
            {{ t('settings.passkeys.remove') }}
          </button>
        </div>
      </div>

      <div v-if="!passkeyAvailable" class="text-xs text-amber-400 mb-3">
        {{ t('settings.passkeys.unsupported') }}
      </div>

      <button v-else @click="addPasskey" :disabled="addingPasskey"
        class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm rounded-lg transition-colors disabled:opacity-50">
        {{ addingPasskey ? t('settings.passkeys.registering') : t('settings.passkeys.add') }}
      </button>

      <div v-if="passkeyMsg" class="text-xs mt-2" :class="passkeyError ? 'text-red-400' : 'text-emerald-400'">{{ passkeyMsg }}</div>
    </section>
  </div>
</template>

<script setup>
import { useConfirm } from '../composables/useConfirm'
import { ref, nextTick, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import QRCode from 'qrcode'
import api from '../api.js'
import LocalePicker from '../components/LocalePicker.vue'
import { formatDate } from '../composables/useFormat.js'
import { enumLabel } from '../composables/useEnumLabel.js'
import { renderApiError } from '../composables/useApiError.js'

const { t } = useI18n()

const user = ref(null)

// Both are stored enums the template used to render raw — display text no
// template scanner can see, and untranslatable as stored.
const roleLabel = (role) => enumLabel('role', role)
const permissionLabel = (perm) => enumLabel('permissions', perm || 'read-write')

// Personal Access Tokens
const tokens = ref([])
const tokenName = ref('')
const tokenPermissions = ref('read-write')
// The <option> labels are the stored values' catalogue entries, resolved in a
// computed so switching locale relabels the open select.
const tokenPermissionOptions = computed(() =>
  ['read-write', 'read', 'write'].map((value) => ({ value, label: enumLabel('permissions', value) })),
)
const creatingToken = ref(false)
const newTokenValue = ref('')
const tokenMsg = ref('')
const tokenError = ref(false)

// Passkeys
const passkeys = ref([])
const addingPasskey = ref(false)
const passkeyMsg = ref('')
const passkeyError = ref(false)
const passkeyAvailable = computed(() => typeof window !== 'undefined' && window.PublicKeyCredential !== undefined)

// Profile
const localeMsg = ref('')
const profileName = ref('')
const savingName = ref(false)
const nameMsg = ref('')
const nameError = ref(false)

// Password
const currentPassword = ref('')
const newPassword = ref('')
const changingPw = ref(false)
const pwMsg = ref('')
const pwError = ref(false)

// Change email
const newEmail = ref('')
const emailCurrentPassword = ref('')
const emailOtp = ref('')
const changingEmail = ref(false)
const cancellingEmail = ref(false)
const emailMsg = ref('')
const emailError = ref(false)

// OTP
const otpEnabled = ref(false)
const otpSecret = ref('')
const otpURI = ref('')
const otpCode = ref('')
const disableCode = ref('')
const settingUpOTP = ref(false)
const verifyingOTP = ref(false)
const disablingOTP = ref(false)
const otpMsg = ref('')
const otpError = ref(false)
const copied = ref(false)
const qrCanvas = ref(null)

onMounted(async () => {
  try {
    user.value = await api.getMe()
    profileName.value = user.value?.name || ''
    otpEnabled.value = user.value?.otp_enabled || false
  } catch { /* redirect handled by router guard */ }
  await Promise.all([loadPasskeys(), loadTokens()])
})

async function saveName() {
  savingName.value = true
  nameMsg.value = ''
  try {
    await api.putJSON('/api/v1/auth/profile', { name: profileName.value })
    nameMsg.value = t('settings.profile.name_updated')
    nameError.value = false
  } catch (e) {
    nameMsg.value = renderApiError(e)
    nameError.value = true
  } finally {
    savingName.value = false
  }
}

async function changePassword() {
  changingPw.value = true
  pwMsg.value = ''
  try {
    await api.putJSON('/api/v1/auth/password', {
      current_password: currentPassword.value,
      new_password: newPassword.value,
    })
    pwMsg.value = t('settings.password.changed')
    pwError.value = false
    currentPassword.value = ''
    newPassword.value = ''
  } catch (e) {
    pwMsg.value = renderApiError(e)
    pwError.value = true
  } finally {
    changingPw.value = false
  }
}

async function changeEmail() {
  changingEmail.value = true
  emailMsg.value = ''
  try {
    const res = await api.putJSON('/api/v1/auth/email', {
      new_email: newEmail.value.trim(),
      current_password: emailCurrentPassword.value,
      otp: emailOtp.value,
    })
    emailMsg.value = t('settings.email.sent')
    emailError.value = false
    emailCurrentPassword.value = ''
    emailOtp.value = ''
    // Reflect the pending change immediately.
    user.value = await api.getMe()
    newEmail.value = ''
  } catch (e) {
    emailMsg.value = renderApiError(e)
    emailError.value = true
  } finally {
    changingEmail.value = false
  }
}

async function cancelEmailChange() {
  cancellingEmail.value = true
  emailMsg.value = ''
  try {
    await api.deleteJSON('/api/v1/auth/email')
    user.value = await api.getMe()
    emailMsg.value = t('settings.email.cancelled')
    emailError.value = false
  } catch (e) {
    emailMsg.value = renderApiError(e)
    emailError.value = true
  } finally {
    cancellingEmail.value = false
  }
}

async function setupOTP() {
  settingUpOTP.value = true
  otpMsg.value = ''
  try {
    const res = await api.postJSON('/api/v1/auth/otp/setup', {})
    otpSecret.value = res.secret
    otpURI.value = res.uri
    await nextTick()
    if (qrCanvas.value) {
      QRCode.toCanvas(qrCanvas.value, res.uri, { width: 200, margin: 0 })
    }
  } catch (e) {
    otpMsg.value = renderApiError(e)
    otpError.value = true
  } finally {
    settingUpOTP.value = false
  }
}

async function verifyOTP() {
  verifyingOTP.value = true
  otpMsg.value = ''
  try {
    await api.postJSON('/api/v1/auth/otp/verify', { code: otpCode.value })
    otpEnabled.value = true
    otpSecret.value = ''
    otpURI.value = ''
    otpCode.value = ''
    otpMsg.value = t('settings.otp.enabled_message')
    otpError.value = false
  } catch (e) {
    otpMsg.value = renderApiError(e)
    otpError.value = true
  } finally {
    verifyingOTP.value = false
  }
}

async function disableOTP() {
  disablingOTP.value = true
  otpMsg.value = ''
  try {
    await api.deleteJSON('/api/v1/auth/otp', { code: disableCode.value })
    otpEnabled.value = false
    disableCode.value = ''
    otpMsg.value = t('settings.otp.disabled_message')
    otpError.value = false
  } catch (e) {
    otpMsg.value = renderApiError(e)
    otpError.value = true
  } finally {
    disablingOTP.value = false
  }
}

function cancelOTPSetup() {
  otpSecret.value = ''
  otpURI.value = ''
  otpCode.value = ''
}

async function copySecret() {
  await navigator.clipboard.writeText(otpSecret.value)
  copied.value = true
  setTimeout(() => { copied.value = false }, 2000)
}

// ---- Token helpers ----

async function loadTokens() {
  try {
    const data = await api.getMyAPIKeys()
    tokens.value = Array.isArray(data) ? data : (data?.data || [])
  } catch { /* ignore */ }
}

async function createToken() {
  creatingToken.value = true
  tokenMsg.value = ''
  newTokenValue.value = ''
  try {
    const result = await api.createMyAPIKey({ name: tokenName.value, permissions: tokenPermissions.value })
    newTokenValue.value = result.token || ''
    tokenMsg.value = t('settings.tokens.created')
    tokenError.value = false
    tokenName.value = ''
    await loadTokens()
  } catch (e) {
    tokenMsg.value = renderApiError(e)
    tokenError.value = true
  } finally {
    creatingToken.value = false
  }
}

async function revokeToken(tok) {
  if (!await useConfirm().ask(t('settings.tokens.revoke_confirm', { name: tok.name }))) return
  tokenMsg.value = ''
  try {
    await api.revokeMyAPIKey(tok.id)
    await loadTokens()
    tokenMsg.value = t('settings.tokens.revoked_message')
    tokenError.value = false
  } catch (e) {
    tokenMsg.value = renderApiError(e)
    tokenError.value = true
  }
}

// ---- Passkey helpers ----

function bufferToBase64url(buffer) {
  const bytes = new Uint8Array(buffer)
  let str = ''
  for (const b of bytes) str += String.fromCharCode(b)
  return btoa(str).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')
}

function base64urlToBuffer(base64url) {
  const base64 = base64url.replace(/-/g, '+').replace(/_/g, '/')
  const str = atob(base64)
  const bytes = new Uint8Array(str.length)
  for (let i = 0; i < str.length; i++) bytes[i] = str.charCodeAt(i)
  return bytes.buffer
}

async function loadPasskeys() {
  try {
    const data = await api.listPasskeys()
    passkeys.value = (Array.isArray(data) ? data : (data?.data || [])).map(pk => ({ ...pk, _editing: false, _editName: '' }))
  } catch { /* ignore */ }
}

async function addPasskey() {
  addingPasskey.value = true
  passkeyMsg.value = ''
  try {
    // Step 1: Begin registration
    const options = await api.passkeyRegisterBegin()

    // Convert base64url fields to ArrayBuffer
    if (options.publicKey) {
      if (options.publicKey.challenge) {
        options.publicKey.challenge = base64urlToBuffer(options.publicKey.challenge)
      }
      if (options.publicKey.user?.id) {
        options.publicKey.user.id = base64urlToBuffer(options.publicKey.user.id)
      }
      if (options.publicKey.excludeCredentials) {
        options.publicKey.excludeCredentials = options.publicKey.excludeCredentials.map(c => ({
          ...c,
          id: base64urlToBuffer(c.id),
        }))
      }
    }

    // Step 2: Create credential
    const credential = await navigator.credentials.create({
      publicKey: options.publicKey,
    })

    // Step 3: Encode for server
    const credentialData = {
      id: credential.id,
      rawId: bufferToBase64url(credential.rawId),
      type: credential.type,
      response: {
        attestationObject: bufferToBase64url(credential.response.attestationObject),
        clientDataJSON: bufferToBase64url(credential.response.clientDataJSON),
      },
    }

    // Step 4: Complete registration
    await api.passkeyRegisterComplete(credentialData)

    passkeyMsg.value = t('settings.passkeys.added_message')
    passkeyError.value = false
    await loadPasskeys()
  } catch (e) {
    if (e.name === 'NotAllowedError') {
      passkeyMsg.value = t('settings.passkeys.cancelled')
    } else {
      passkeyMsg.value = renderApiError(e) || t('settings.passkeys.error_add')
    }
    passkeyError.value = true
  } finally {
    addingPasskey.value = false
  }
}

async function deletePasskey(pk) {
  if (!await useConfirm().ask(t('settings.passkeys.remove_confirm'))) return
  passkeyMsg.value = ''
  try {
    await api.deletePasskey(pk.id)
    passkeys.value = passkeys.value.filter(p => p.id !== pk.id)
    passkeyMsg.value = t('settings.passkeys.removed')
    passkeyError.value = false
  } catch (e) {
    passkeyMsg.value = renderApiError(e)
    passkeyError.value = true
  }
}

function startPasskeyRename(pk) {
  pk._editing = true
  pk._editName = pk.name || t('settings.passkeys.default_name')
}

async function savePasskeyName(pk) {
  try {
    await api.renamePasskey(pk.id, pk._editName)
    pk.name = pk._editName
    pk._editing = false
  } catch (e) {
    passkeyMsg.value = renderApiError(e)
    passkeyError.value = true
  }
}
</script>
