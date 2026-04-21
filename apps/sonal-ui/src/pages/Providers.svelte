<script lang="ts">
  import { onMount } from 'svelte'
  import { createSonalAgentApi } from '../lib/agentapi/client'
  import { authStore } from '../lib/auth/auth-store.svelte'
  import type { ProviderResponse, CreateProviderRequest, UpdateProviderRequest, ModelConfig } from '../lib/agentapi/types'

  const agentBaseUrl = import.meta.env.VITE_AGENT_API_BASE_URL ?? '/api/v1/runtime'

  const agentApi = $derived.by(() =>
    createSonalAgentApi({ baseUrl: agentBaseUrl, accessToken: authStore.accessToken }),
  )

  type FormMode = 'add' | 'edit'

  let providers = $state<ProviderResponse[]>([])
  let loading = $state(false)
  let error = $state<string | null>(null)
  let formVisible = $state(false)
  let formMode = $state<FormMode>('add')
  let formError = $state<string | null>(null)
  let formSubmitting = $state(false)
  let deleteTarget = $state<string | null>(null)
  let deleteSubmitting = $state(false)

  let fieldName = $state('')
  let fieldType = $state('openai-compatible')
  let fieldDisplayName = $state('')
  let fieldBaseUrl = $state('')
  let fieldApiKey = $state('')
  let fieldModels = $state<ModelConfig[]>([])

  async function loadProviders() {
    loading = true
    error = null
    try {
      const result = await agentApi.listProviders()
      providers = result.providers
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load providers'
    } finally {
      loading = false
    }
  }

  onMount(() => {
    loadProviders()
  })

  function openAddForm() {
    formMode = 'add'
    fieldName = ''
    fieldType = 'openai-compatible'
    fieldDisplayName = ''
    fieldBaseUrl = ''
    fieldApiKey = ''
    fieldModels = []
    formError = null
    formVisible = true
  }

  function openEditForm(provider: ProviderResponse) {
    formMode = 'edit'
    fieldName = provider.name
    fieldType = provider.type
    fieldDisplayName = provider.displayName ?? ''
    fieldBaseUrl = provider.baseUrl
    fieldApiKey = ''
    fieldModels = provider.models.map((m) => ({
      name: m.name,
      displayName: m.displayName,
      summarization: m.summarization ?? false,
    }))
    formError = null
    formVisible = true
  }

  function addModel() {
    fieldModels = [...fieldModels, { name: '', displayName: '', summarization: false }]
  }

  function removeModel(index: number) {
    fieldModels = fieldModels.filter((_, i) => i !== index)
  }

  function updateModelField<K extends keyof ModelConfig>(index: number, field: K, value: ModelConfig[K]) {
    fieldModels = fieldModels.map((m, i) => (i === index ? { ...m, [field]: value } : m))
  }

  function cancelForm() {
    formVisible = false
    formError = null
  }

  async function handleFormSubmit(e: Event) {
    e.preventDefault()
    formError = null
    formSubmitting = true
    try {
      const models = fieldModels
        .filter((m) => m.name.trim())
        .map((m) => ({
          name: m.name.trim(),
          summarization: m.summarization,
          ...(m.displayName?.trim() ? { displayName: m.displayName.trim() } : {}),
        }))
      if (formMode === 'add') {
        const body: CreateProviderRequest = {
          name: fieldName,
          type: fieldType,
          baseUrl: fieldBaseUrl,
          apiKey: fieldApiKey,
          ...(fieldDisplayName ? { displayName: fieldDisplayName } : {}),
          models,
        }
        await agentApi.createProvider({ body })
      } else {
        const body: UpdateProviderRequest = {
          baseUrl: fieldBaseUrl,
          ...(fieldDisplayName ? { displayName: fieldDisplayName } : {}),
          ...(fieldApiKey ? { apiKey: fieldApiKey } : {}),
          models,
        }
        await agentApi.updateProvider({
          providerName: fieldName,
          body,
        })
      }
      formVisible = false
      await loadProviders()
    } catch (err) {
      formError = err instanceof Error ? err.message : 'Failed to save provider'
    } finally {
      formSubmitting = false
    }
  }

  function requestDelete(providerName: string) {
    deleteTarget = providerName
  }

  function cancelDelete() {
    deleteTarget = null
  }

  async function confirmDelete() {
    if (!deleteTarget) return
    deleteSubmitting = true
    error = null
    const name = deleteTarget
    try {
      await agentApi.deleteProvider({ providerName: name })
      deleteTarget = null
      await loadProviders()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to delete provider'
      deleteTarget = null
    } finally {
      deleteSubmitting = false
    }
  }
</script>

<section class="page" aria-labelledby="providers-heading">
  <header class="page-header">
    <h1 id="providers-heading">Providers</h1>
    <button type="button" class="primary" onclick={openAddForm} disabled={formVisible}>
      Add Provider
    </button>
  </header>

  {#if error}
    <p class="error" role="alert">{error}</p>
  {/if}

  {#if formVisible}
    <form class="provider-form" onsubmit={handleFormSubmit} aria-label="Provider form">
      <h2 class="form-title">{formMode === 'add' ? 'Add Provider' : 'Edit Provider'}</h2>

      {#if formMode === 'add'}
        <div class="field">
          <label for="field-name">Name</label>
          <input
            id="field-name"
            type="text"
            bind:value={fieldName}
            disabled={formSubmitting}
            placeholder="e.g. openai"
            required
          />
        </div>

        <div class="field">
          <label for="field-type">Type</label>
          <select id="field-type" bind:value={fieldType} disabled={formSubmitting}>
            <option value="openai-compatible">openai-compatible</option>
          </select>
        </div>
      {/if}

      <div class="field">
        <label for="field-display-name">Display Name</label>
        <input
          id="field-display-name"
          type="text"
          bind:value={fieldDisplayName}
          disabled={formSubmitting}
          placeholder="e.g. OpenAI"
        />
      </div>

      <div class="field">
        <label for="field-base-url">Base URL</label>
        <input
          id="field-base-url"
          type="url"
          bind:value={fieldBaseUrl}
          disabled={formSubmitting}
          placeholder="https://api.openai.com/v1"
          required
        />
      </div>

      <div class="field">
        <label for="field-api-key">API Key{formMode === 'edit' ? ' (leave blank to keep current)' : ''}</label>
        <input
          id="field-api-key"
          type="password"
          bind:value={fieldApiKey}
          disabled={formSubmitting}
          placeholder={formMode === 'edit' ? '••••••••' : ''}
          required={formMode === 'add'}
        />
      </div>

      <div class="models-section">
        <div class="models-header">
          <span class="models-label">Models</span>
          <button type="button" class="secondary add-model-btn" onclick={addModel} disabled={formSubmitting}>
            Add Model
          </button>
        </div>
        {#each fieldModels as model, i (i)}
          <div class="model-entry">
            <div class="model-entry-row">
              <input
                type="text"
                value={model.name}
                oninput={(e) => updateModelField(i, 'name', (e.target as HTMLInputElement).value)}
                disabled={formSubmitting}
                placeholder="e.g. gpt-4.1"
                aria-label="Model name"
              />
              <input
                type="text"
                value={model.displayName ?? ''}
                oninput={(e) => updateModelField(i, 'displayName', (e.target as HTMLInputElement).value)}
                disabled={formSubmitting}
                placeholder="e.g. GPT 4.1"
                aria-label="Model display name"
              />
              <button
                type="button"
                class="action-btn danger-text"
                onclick={() => removeModel(i)}
                disabled={formSubmitting}
                aria-label="Remove model"
              >
                ✕
              </button>
            </div>
            <div class="model-summarization">
              <label class="summarization-label">
                <input
                  type="checkbox"
                  checked={model.summarization}
                  onchange={(e) =>
                    updateModelField(i, 'summarization', (e.target as HTMLInputElement).checked)}
                  disabled={formSubmitting}
                />
                Summarization
              </label>
              <p class="summarization-hint">
                Use this model for summarization tasks (e.g. session titles). Prefer fast, inexpensive models.
              </p>
            </div>
          </div>
        {/each}
      </div>

      {#if formError}
        <p class="error" role="alert">{formError}</p>
      {/if}

      <div class="form-actions">
        <button type="submit" class="primary" disabled={formSubmitting}>
          {formSubmitting ? 'Saving…' : 'Save'}
        </button>
        <button type="button" class="secondary" onclick={cancelForm} disabled={formSubmitting}>
          Cancel
        </button>
      </div>
    </form>
  {/if}

  {#if deleteTarget}
    <div class="confirm-dialog" role="dialog" aria-modal="true" aria-label="Confirm delete">
      <p>Delete provider <strong>{deleteTarget}</strong>? This cannot be undone.</p>
      <div class="form-actions">
        <button type="button" class="danger" onclick={confirmDelete} disabled={deleteSubmitting}>
          {deleteSubmitting ? 'Deleting…' : 'Delete'}
        </button>
        <button type="button" class="secondary" onclick={cancelDelete} disabled={deleteSubmitting}>
          Cancel
        </button>
      </div>
    </div>
  {/if}

  {#if loading}
    <p class="loading" aria-busy="true">Loading…</p>
  {:else if providers.length === 0 && !formVisible}
    <p class="empty-state">No providers configured yet. Add your first provider to get started.</p>
  {:else if providers.length > 0}
    <table class="providers-table">
      <thead>
        <tr>
          <th>Name</th>
          <th>Type</th>
          <th>Base URL</th>
          <th>API Key</th>
          <th>Models</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        {#each providers as provider (provider.name)}
          <tr>
            <td>
              <span class="provider-display-name">{provider.displayName ?? provider.name}</span>
              {#if provider.displayName}
                <span class="provider-technical-name">{provider.name}</span>
              {/if}
            </td>
            <td>{provider.type}</td>
            <td class="url-cell">{provider.baseUrl}</td>
            <td><code>{provider.apiKeyPreview}</code></td>
            <td class="models-count">{provider.models.length} {provider.models.length === 1 ? 'model' : 'models'}</td>
            <td class="actions-cell">
              <div class="actions-cell-inner">
                <button
                  type="button"
                  class="action-btn"
                  onclick={() => openEditForm(provider)}
                  disabled={formVisible || !!deleteTarget}
                >
                  Edit
                </button>
                <button
                  type="button"
                  class="action-btn danger-text"
                  onclick={() => requestDelete(provider.name)}
                  disabled={formVisible || !!deleteTarget}
                >
                  Delete
                </button>
              </div>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</section>

<style>
  .page {
    width: 100%;
    max-width: none;
  }

  .page-header {
    display: flex;
    align-items: center;
    gap: var(--space-16);
    margin-bottom: var(--space-20);
    flex-wrap: wrap;
  }

  h1 {
    font-size: var(--font-size-h1);
    font-weight: 700;
    color: var(--text-h);
    margin: 0;
  }

  .loading,
  .empty-state {
    color: var(--text);
    font-size: var(--font-size-body);
    line-height: 1.5;
    margin: 0;
  }

  .error {
    margin: 0 0 var(--space-16);
    padding: var(--space-8) var(--space-12);
    border-radius: var(--radius-default);
    background: var(--danger-bg);
    border: 1px solid var(--color-danger);
    color: var(--color-danger);
    font-size: var(--font-size-caption);
  }

  .provider-form,
  .confirm-dialog {
    margin-bottom: var(--space-24);
    padding: var(--space-20);
    border: 1px solid var(--border);
    border-radius: var(--radius-default);
    background: var(--surface-raised);
    display: flex;
    flex-direction: column;
    gap: var(--space-12);
    max-width: 32rem;
  }

  .form-title {
    font-size: var(--font-size-h2);
    font-weight: 700;
    color: var(--text-on-raised);
    margin: 0;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: var(--space-8);
  }

  label {
    font-size: var(--font-size-caption);
    font-weight: 500;
    line-height: 2;
    color: var(--text);
  }

  input,
  select {
    font: inherit;
    font-weight: 400;
    padding: var(--space-12) var(--space-16);
    border-radius: var(--radius-input);
    border: 1px solid var(--border);
    background: var(--color-input-bg);
    color: var(--color-input-text);
  }

  input:focus-visible,
  select:focus-visible {
    outline: 1px solid var(--color-accent-blue);
    outline-offset: 0;
  }

  input:disabled,
  select:disabled {
    opacity: 0.55;
  }

  .form-actions {
    display: flex;
    gap: var(--space-8);
    flex-wrap: wrap;
  }

  .action-btn {
    font: inherit;
    font-weight: 500;
    cursor: pointer;
    padding: var(--space-4) var(--space-12);
    border-radius: var(--radius-default);
    border: 1px solid var(--color-border-outline);
    background: transparent;
    color: var(--text-h);
    font-size: var(--font-size-caption);
  }

  .action-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .action-btn.danger-text {
    color: var(--color-danger);
    border-color: var(--danger-border);
  }

  .providers-table {
    width: 100%;
    border-collapse: collapse;
    font-size: var(--font-size-caption);
  }

  .providers-table th,
  .providers-table td {
    text-align: left;
    vertical-align: middle;
    padding: var(--space-12) var(--space-12);
    border-bottom: 1px solid var(--border);
  }

  .providers-table th {
    color: var(--text);
    font-weight: 700;
    font-size: var(--font-size-caption);
    text-transform: none;
    letter-spacing: normal;
  }

  .provider-display-name {
    display: block;
    color: var(--text-h);
    font-weight: 500;
  }

  .provider-technical-name {
    display: block;
    color: var(--text);
    font-size: var(--font-size-caption);
  }

  .url-cell {
    max-width: 16rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text);
  }

  code {
    font-family: var(--mono);
    font-size: var(--font-size-caption);
    color: var(--text);
  }

  /* Inner wrapper: flex on <td> breaks row borders / vertical alignment in some engines. */
  .actions-cell-inner {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-8);
    align-items: center;
  }

  .confirm-dialog p {
    margin: 0;
    color: var(--text-on-raised);
    font-size: var(--font-size-body);
    line-height: 1.5;
  }

  .models-section {
    display: flex;
    flex-direction: column;
    gap: var(--space-8);
  }

  .models-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .models-label {
    font-size: var(--font-size-caption);
    color: var(--text);
    font-weight: 500;
  }

  .add-model-btn {
    font-size: var(--font-size-caption);
    padding: var(--space-4) var(--space-12);
  }

  .model-entry {
    display: flex;
    flex-direction: column;
    gap: var(--space-8);
  }

  .model-entry-row {
    display: flex;
    gap: var(--space-8);
    align-items: center;
  }

  .model-entry-row input[type='text'] {
    flex: 1;
  }

  .model-summarization {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    padding-left: var(--space-4);
  }

  .summarization-label {
    display: inline-flex;
    align-items: center;
    gap: var(--space-8);
    font-size: var(--font-size-caption);
    font-weight: 500;
    color: var(--text);
    cursor: pointer;
  }

  .summarization-label input[type='checkbox'] {
    width: auto;
    margin: 0;
    cursor: inherit;
  }

  .summarization-hint {
    margin: 0;
    font-size: var(--font-size-caption);
    line-height: 1.45;
    color: var(--text-muted);
  }

  .models-count {
    color: var(--text);
    font-size: var(--font-size-caption);
  }
</style>
