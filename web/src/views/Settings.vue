<template>
  <div class="max-w-2xl mx-auto px-6 py-8 space-y-8">
    <h1 class="text-xl font-bold text-white">Settings</h1>

    <!-- Profile -->
    <section class="bg-slate-900 border border-slate-800 rounded-xl p-6">
      <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider mb-4">Profile</h2>
      <div class="space-y-4">
        <div>
          <label class="block text-xs text-slate-500 mb-1">Email</label>
          <div class="text-sm text-slate-300">{{ user?.email }}</div>
        </div>
        <div>
          <label class="block text-xs text-slate-500 mb-1">Name</label>
          <div class="flex gap-2 items-center">
            <input v-model="profileName" type="text"
              class="flex-1 px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500" />
            <button @click="saveName" :disabled="savingName"
              class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm rounded-lg transition-colors disabled:opacity-50">
              {{ savingName ? 'Saving...' : 'Save' }}
            </button>
          </div>
          <div v-if="nameMsg" class="text-xs mt-1" :class="nameError ? 'text-red-400' : 'text-emerald-400'">{{ nameMsg }}</div>
        </div>
        <div>
          <label class="block text-xs text-slate-500 mb-1">Role</label>
          <span class="inline-block px-2 py-0.5 text-xs font-medium rounded-full"
            :class="user?.role === 'admin' ? 'bg-purple-500/20 text-purple-300' :
                     user?.role === 'manager' ? 'bg-blue-500/20 text-blue-300' :
                     'bg-slate-500/20 text-slate-400'">
            {{ user?.role }}
          </span>
        </div>
      </div>
    </section>

    <!-- Change Password (only for local login users) -->
    <section v-if="user?.has_password" class="bg-slate-900 border border-slate-800 rounded-xl p-6">
      <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider mb-4">Password</h2>
      <div class="space-y-3">
        <div>
          <label class="block text-xs text-slate-500 mb-1">Current password</label>
          <input v-model="currentPassword" type="password"
            class="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500" />
        </div>
        <div>
          <label class="block text-xs text-slate-500 mb-1">New password (min 7 characters)</label>
          <input v-model="newPassword" type="password"
            class="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500" />
        </div>
        <button @click="changePassword" :disabled="changingPw"
          class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm rounded-lg transition-colors disabled:opacity-50">
          {{ changingPw ? 'Changing...' : 'Change password' }}
        </button>
        <div v-if="pwMsg" class="text-xs" :class="pwError ? 'text-red-400' : 'text-emerald-400'">{{ pwMsg }}</div>
      </div>
    </section>

    <!-- Two-Factor Authentication -->
    <section class="bg-slate-900 border border-slate-800 rounded-xl p-6">
      <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider mb-4">Two-Factor Authentication (OTP)</h2>

      <!-- OTP enabled -->
      <div v-if="otpEnabled" class="space-y-3">
        <div class="flex items-center gap-2">
          <span class="inline-block w-2 h-2 rounded-full bg-emerald-400"></span>
          <span class="text-sm text-emerald-400">OTP is enabled</span>
        </div>
        <p class="text-xs text-slate-500">Your account is protected with a time-based one-time password.</p>
        <button @click="disableOTP" :disabled="disablingOTP"
          class="px-4 py-2 bg-red-600/20 hover:bg-red-600/30 text-red-400 text-sm rounded-lg border border-red-600/30 transition-colors disabled:opacity-50">
          {{ disablingOTP ? 'Disabling...' : 'Disable OTP' }}
        </button>
      </div>

      <!-- OTP setup flow -->
      <div v-else class="space-y-4">
        <div v-if="!otpSecret" class="space-y-3">
          <p class="text-sm text-slate-400">Add an extra layer of security to your account with a TOTP authenticator app.</p>
          <button @click="setupOTP" :disabled="settingUpOTP"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm rounded-lg transition-colors disabled:opacity-50">
            {{ settingUpOTP ? 'Setting up...' : 'Enable OTP' }}
          </button>
        </div>

        <!-- Step 2: Show QR + secret + verify -->
        <div v-else class="space-y-4">
          <p class="text-sm text-slate-400">Scan this QR code with your authenticator app (Google Authenticator, Authy, etc.):</p>

          <!-- QR Code -->
          <div class="flex justify-center">
            <div class="bg-white p-4 rounded-xl">
              <canvas ref="qrCanvas"></canvas>
            </div>
          </div>

          <!-- Secret for manual entry -->
          <div>
            <label class="block text-xs text-slate-500 mb-1">Or enter this key manually</label>
            <div class="flex items-center gap-2">
              <code class="flex-1 px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-amber-300 font-mono tracking-wider select-all">
                {{ otpSecret }}
              </code>
              <button @click="copySecret"
                class="px-3 py-2 bg-slate-700 hover:bg-slate-600 text-slate-300 text-sm rounded-lg transition-colors">
                {{ copied ? 'Copied' : 'Copy' }}
              </button>
            </div>
          </div>

          <!-- Verify code -->
          <div>
            <label class="block text-xs text-slate-500 mb-1">Enter the 6-digit code from your app to verify</label>
            <div class="flex gap-2 items-center">
              <input v-model="otpCode" type="text" maxlength="6" placeholder="000000"
                class="w-32 px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white font-mono tracking-widest text-center focus:outline-none focus:border-blue-500"
                @keyup.enter="verifyOTP" />
              <button @click="verifyOTP" :disabled="verifyingOTP"
                class="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-white text-sm rounded-lg transition-colors disabled:opacity-50">
                {{ verifyingOTP ? 'Verifying...' : 'Verify & Enable' }}
              </button>
            </div>
          </div>

          <button @click="cancelOTPSetup" class="text-xs text-slate-500 hover:text-slate-400">Cancel</button>
        </div>
      </div>

      <div v-if="otpMsg" class="text-xs mt-2" :class="otpError ? 'text-red-400' : 'text-emerald-400'">{{ otpMsg }}</div>
    </section>

    <!-- Personal Access Tokens -->
    <section class="bg-slate-900 border border-slate-800 rounded-xl p-6">
      <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider mb-4">Personal Access Tokens</h2>
      <p class="text-sm text-slate-400 mb-4">Tokens authenticate CLI tools and AI agents. They work across all your organizations.</p>

      <!-- Create token form -->
      <div class="space-y-3 mb-6">
        <div class="flex gap-2 items-end">
          <div class="flex-1">
            <label class="block text-xs text-slate-500 mb-1">Token name</label>
            <input v-model="tokenName" type="text" placeholder="e.g. claude-agent"
              class="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500" />
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1">Permissions</label>
            <select v-model="tokenPermissions"
              class="px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-white focus:outline-none focus:border-blue-500">
              <option value="read-write">Read & Write</option>
              <option value="read">Read only</option>
              <option value="write">Write only</option>
            </select>
          </div>
          <button @click="createToken" :disabled="creatingToken || !tokenName"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm rounded-lg transition-colors disabled:opacity-50">
            {{ creatingToken ? 'Creating...' : 'Create token' }}
          </button>
        </div>
        <div v-if="newTokenValue" class="p-3 bg-emerald-950/40 border border-emerald-900/50 rounded-lg">
          <p class="text-xs text-emerald-400 mb-1">Token created. Copy it now -- you won't see it again.</p>
          <code class="block text-sm text-emerald-300 font-mono break-all select-all">{{ newTokenValue }}</code>
        </div>
        <div v-if="tokenMsg" class="text-xs" :class="tokenError ? 'text-red-400' : 'text-emerald-400'">{{ tokenMsg }}</div>
      </div>

      <!-- Token list -->
      <div v-if="tokens.length > 0" class="space-y-2">
        <div v-for="t in tokens" :key="t.id"
          class="flex items-center gap-3 px-4 py-3 bg-slate-800 border border-slate-700 rounded-lg">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2">
              <span class="text-sm font-medium text-slate-200">{{ t.name }}</span>
              <span class="text-xs px-1.5 py-0.5 rounded bg-slate-700 text-slate-400">{{ t.permissions || 'read-write' }}</span>
              <span v-if="t.revoked_at" class="text-xs px-1.5 py-0.5 rounded bg-red-500/20 text-red-400">revoked</span>
            </div>
            <div class="text-xs text-slate-500 mt-0.5">
              Created {{ t.created_at ? (typeof t.created_at === 'number' ? new Date(t.created_at * 1000) : new Date(t.created_at)).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' }) : 'Unknown' }}
              <span v-if="t.last_used_at" class="ml-2">Last used {{ (typeof t.last_used_at === 'number' ? new Date(t.last_used_at * 1000) : new Date(t.last_used_at)).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' }) }}</span>
              <span v-else class="ml-2">Never used</span>
            </div>
          </div>
          <button v-if="!t.revoked_at" @click="revokeToken(t)"
            class="text-xs text-red-400 hover:text-red-300 px-2 py-1 rounded hover:bg-red-500/10 transition-colors flex-shrink-0">
            Revoke
          </button>
        </div>
      </div>
      <div v-else class="text-sm text-slate-600">No tokens yet.</div>
    </section>

    <!-- Passkeys -->
    <section class="bg-slate-900 border border-slate-800 rounded-xl p-6">
      <h2 class="text-sm font-semibold text-slate-400 uppercase tracking-wider mb-4">Passkeys</h2>
      <p class="text-sm text-slate-400 mb-4">Use a fingerprint, face, or security key instead of a password.</p>

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
                <button @click="savePasskeyName(pk)" class="text-xs text-blue-400 hover:text-blue-300">Save</button>
                <button @click="pk._editing = false" class="text-xs text-slate-500 hover:text-slate-400">Cancel</button>
              </div>
            </template>
            <template v-else>
              <div class="flex items-center gap-2">
                <span class="text-sm font-medium text-slate-200">{{ pk.name || 'Passkey' }}</span>
                <button @click="pk._editing = true; pk._editName = pk.name || 'Passkey'"
                  class="text-slate-600 hover:text-slate-400 transition-colors">
                  <svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
                  </svg>
                </button>
              </div>
              <div class="text-xs text-slate-500 mt-0.5">
                Added {{ pk.created_at ? (typeof pk.created_at === 'number' ? new Date(pk.created_at * 1000) : new Date(pk.created_at)).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' }) : 'Unknown' }}
                <span v-if="pk.last_used_at" class="ml-2">Last used {{ (typeof pk.last_used_at === 'number' ? new Date(pk.last_used_at * 1000) : new Date(pk.last_used_at)).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' }) }}</span>
              </div>
            </template>
          </div>
          <button @click="deletePasskey(pk)"
            class="text-xs text-red-400 hover:text-red-300 px-2 py-1 rounded hover:bg-red-500/10 transition-colors flex-shrink-0">
            Remove
          </button>
        </div>
      </div>

      <div v-if="!passkeyAvailable" class="text-xs text-amber-400 mb-3">
        Passkeys are not supported by this browser.
      </div>

      <button v-else @click="addPasskey" :disabled="addingPasskey"
        class="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm rounded-lg transition-colors disabled:opacity-50">
        {{ addingPasskey ? 'Registering...' : 'Add passkey' }}
      </button>

      <div v-if="passkeyMsg" class="text-xs mt-2" :class="passkeyError ? 'text-red-400' : 'text-emerald-400'">{{ passkeyMsg }}</div>
    </section>
  </div>
</template>

<script setup>
import { useConfirm } from '../composables/useConfirm'
import { ref, nextTick, onMounted, computed } from 'vue'
import QRCode from 'qrcode'
import api from '../api.js'

const user = ref(null)

// Personal Access Tokens
const tokens = ref([])
const tokenName = ref('')
const tokenPermissions = ref('read-write')
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

// OTP
const otpEnabled = ref(false)
const otpSecret = ref('')
const otpURI = ref('')
const otpCode = ref('')
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
    nameMsg.value = 'Name updated'
    nameError.value = false
  } catch (e) {
    nameMsg.value = e.message
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
    pwMsg.value = 'Password changed'
    pwError.value = false
    currentPassword.value = ''
    newPassword.value = ''
  } catch (e) {
    pwMsg.value = e.message
    pwError.value = true
  } finally {
    changingPw.value = false
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
    otpMsg.value = e.message
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
    otpMsg.value = 'OTP enabled successfully'
    otpError.value = false
  } catch (e) {
    otpMsg.value = e.message
    otpError.value = true
  } finally {
    verifyingOTP.value = false
  }
}

async function disableOTP() {
  disablingOTP.value = true
  otpMsg.value = ''
  try {
    await api.deleteJSON('/api/v1/auth/otp')
    otpEnabled.value = false
    otpMsg.value = 'OTP disabled'
    otpError.value = false
  } catch (e) {
    otpMsg.value = e.message
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
    tokenMsg.value = 'Token created'
    tokenError.value = false
    tokenName.value = ''
    await loadTokens()
  } catch (e) {
    tokenMsg.value = e.message
    tokenError.value = true
  } finally {
    creatingToken.value = false
  }
}

async function revokeToken(t) {
  if (!await useConfirm().ask(`Revoke token "${t.name}"?`, 'Confirm')) return
  tokenMsg.value = ''
  try {
    await api.revokeMyAPIKey(t.id)
    await loadTokens()
    tokenMsg.value = 'Token revoked'
    tokenError.value = false
  } catch (e) {
    tokenMsg.value = e.message
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

    passkeyMsg.value = 'Passkey added successfully'
    passkeyError.value = false
    await loadPasskeys()
  } catch (e) {
    if (e.name === 'NotAllowedError') {
      passkeyMsg.value = 'Passkey registration was cancelled'
    } else {
      passkeyMsg.value = e.message || 'Failed to add passkey'
    }
    passkeyError.value = true
  } finally {
    addingPasskey.value = false
  }
}

async function deletePasskey(pk) {
  if (!await useConfirm().ask('Remove this passkey?', 'Confirm')) return
  passkeyMsg.value = ''
  try {
    await api.deletePasskey(pk.id)
    passkeys.value = passkeys.value.filter(p => p.id !== pk.id)
    passkeyMsg.value = 'Passkey removed'
    passkeyError.value = false
  } catch (e) {
    passkeyMsg.value = e.message
    passkeyError.value = true
  }
}

async function savePasskeyName(pk) {
  try {
    await api.renamePasskey(pk.id, pk._editName)
    pk.name = pk._editName
    pk._editing = false
  } catch (e) {
    passkeyMsg.value = e.message
    passkeyError.value = true
  }
}
</script>
