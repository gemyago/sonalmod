import { describe, it, expect, beforeEach, vi } from 'vitest'
import { faker } from '@faker-js/faker'
import { render, screen, waitFor, within } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import Providers from './Providers.svelte'
import type { ModelConfig, ProviderResponse } from '../lib/agentapi/types'

const mocks = vi.hoisted(() => ({
  listProviders: vi.fn(),
  createProvider: vi.fn(),
  updateProvider: vi.fn(),
  deleteProvider: vi.fn(),
}))

vi.mock('../lib/agentapi/client', () => ({
  createSonalAgentApi: vi.fn(() => ({
    listProviders: mocks.listProviders,
    createProvider: mocks.createProvider,
    updateProvider: mocks.updateProvider,
    deleteProvider: mocks.deleteProvider,
  })),
}))

vi.mock('../lib/auth/auth-store.svelte', () => ({
  authStore: { accessToken: null },
}))

function makeProvider(overrides?: Partial<ProviderResponse>): ProviderResponse {
  return {
    name: faker.word.noun().toLowerCase(),
    type: 'openai-compatible',
    displayName: faker.company.name(),
    baseUrl: faker.internet.url(),
    apiKeyPreview: `...${faker.string.alphanumeric(4)}`,
    models: [],
    createdAt: faker.date.recent().toISOString(),
    updatedAt: faker.date.recent().toISOString(),
    ...overrides,
  }
}

function makeModelConfig(overrides?: Partial<ModelConfig>): ModelConfig {
  return {
    name: `${faker.word.noun()}-${faker.string.alphanumeric(4)}`,
    displayName: faker.commerce.productName(),
    summarization: false,
    ...overrides,
  }
}

describe('Providers', () => {
  beforeEach(() => {
    mocks.listProviders.mockReset()
    mocks.createProvider.mockReset()
    mocks.updateProvider.mockReset()
    mocks.deleteProvider.mockReset()
  })

  it('renders heading and Add Provider button', async () => {
    mocks.listProviders.mockResolvedValue({ providers: [] })
    render(Providers)
    expect(screen.getByRole('heading', { name: 'Providers' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Add Provider' })).toBeInTheDocument()
  })

  it('renders empty state when no providers', async () => {
    mocks.listProviders.mockResolvedValue({ providers: [] })
    render(Providers)
    await waitFor(() => {
      expect(
        screen.getByText(/No providers configured yet/),
      ).toBeInTheDocument()
    })
  })

  it('renders provider list with name, type, baseUrl, apiKeyPreview', async () => {
    const provider = makeProvider()
    mocks.listProviders.mockResolvedValue({ providers: [provider] })
    render(Providers)
    await waitFor(() => {
      expect(screen.getByText(provider.displayName!)).toBeInTheDocument()
      expect(screen.getByText(provider.name)).toBeInTheDocument()
      expect(screen.getByText(provider.type)).toBeInTheDocument()
      expect(screen.getByText(provider.apiKeyPreview)).toBeInTheDocument()
    })
  })

  it('add flow: opens form, submits, list refreshes', async () => {
    const user = userEvent.setup()
    const newProvider = makeProvider({ displayName: undefined })
    mocks.listProviders
      .mockResolvedValueOnce({ providers: [] })
      .mockResolvedValueOnce({ providers: [newProvider] })
    mocks.createProvider.mockResolvedValue(newProvider)

    render(Providers)
    await waitFor(() => expect(screen.getByText(/No providers configured yet/)).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: 'Add Provider' }))
    expect(screen.getByRole('form', { name: 'Provider form' })).toBeInTheDocument()

    await user.type(screen.getByLabelText('Name'), newProvider.name)
    await user.type(screen.getByLabelText('Base URL'), newProvider.baseUrl)
    await user.type(screen.getByLabelText('API Key'), faker.string.alphanumeric(32))
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(mocks.createProvider).toHaveBeenCalledOnce()
      expect(mocks.listProviders).toHaveBeenCalledTimes(2)
    })
    await waitFor(() => {
      expect(screen.queryByRole('form', { name: 'Provider form' })).not.toBeInTheDocument()
    })
  })

  it('edit flow: opens form with current values, submits update', async () => {
    const user = userEvent.setup()
    const provider = makeProvider()
    const updatedProvider = { ...provider, displayName: faker.company.name() }
    mocks.listProviders
      .mockResolvedValueOnce({ providers: [provider] })
      .mockResolvedValueOnce({ providers: [updatedProvider] })
    mocks.updateProvider.mockResolvedValue(updatedProvider)

    render(Providers)
    await waitFor(() => expect(screen.getByText(provider.displayName!)).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: 'Edit' }))
    expect(screen.getByRole('form', { name: 'Provider form' })).toBeInTheDocument()

    const displayNameInput = screen.getByLabelText('Display Name')
    await user.clear(displayNameInput)
    await user.type(displayNameInput, updatedProvider.displayName!)

    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(mocks.updateProvider).toHaveBeenCalledOnce()
      expect(mocks.listProviders).toHaveBeenCalledTimes(2)
    })
  })

  it('edit flow sends summarization true on models when saving', async () => {
    const user = userEvent.setup()
    const model = makeModelConfig({ summarization: true })
    const provider = makeProvider({
      models: [model],
    })
    const updatedProvider = { ...provider, updatedAt: faker.date.recent().toISOString() }
    mocks.listProviders
      .mockResolvedValueOnce({ providers: [provider] })
      .mockResolvedValueOnce({ providers: [updatedProvider] })
    mocks.updateProvider.mockResolvedValue(updatedProvider)

    render(Providers)
    await waitFor(() => expect(screen.getByText(provider.displayName!)).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: 'Edit' }))
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(mocks.updateProvider).toHaveBeenCalledWith(
        expect.objectContaining({
          body: expect.objectContaining({
            models: [expect.objectContaining({ name: model.name, summarization: true })],
          }),
        }),
      )
    })
  })

  it('delete flow: shows confirm dialog, confirms and removes provider', async () => {
    const user = userEvent.setup()
    const provider = makeProvider()
    mocks.listProviders
      .mockResolvedValueOnce({ providers: [provider] })
      .mockResolvedValueOnce({ providers: [] })
    mocks.deleteProvider.mockResolvedValue(undefined)

    render(Providers)
    await waitFor(() => expect(screen.getByText(provider.displayName!)).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: 'Delete' }))
    const dialog = screen.getByRole('dialog', { name: 'Confirm delete' })
    expect(dialog).toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: 'Delete' }))

    await waitFor(() => {
      expect(mocks.deleteProvider).toHaveBeenCalledOnce()
      expect(mocks.listProviders).toHaveBeenCalledTimes(2)
    })
    await waitFor(() => {
      expect(screen.getByText(/No providers configured yet/)).toBeInTheDocument()
    })
  })

  it('shows error alert on API load failure', async () => {
    mocks.listProviders.mockRejectedValue(new Error('Network error'))
    render(Providers)
    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent('Network error')
    })
  })

  it('shows form error on create failure', async () => {
    const user = userEvent.setup()
    mocks.listProviders.mockResolvedValue({ providers: [] })
    mocks.createProvider.mockRejectedValue(new Error('Conflict'))

    render(Providers)
    await waitFor(() => expect(screen.getByText(/No providers configured yet/)).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: 'Add Provider' }))
    await user.type(screen.getByLabelText('Name'), faker.word.noun().toLowerCase())
    await user.type(screen.getByLabelText('Base URL'), faker.internet.url())
    await user.type(screen.getByLabelText('API Key'), faker.string.alphanumeric(32))
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent('Conflict')
    })
    expect(screen.getByRole('form', { name: 'Provider form' })).toBeInTheDocument()
  })

  it('create provider form shows models section with Add Model button', async () => {
    const user = userEvent.setup()
    mocks.listProviders.mockResolvedValue({ providers: [] })

    render(Providers)
    await waitFor(() => expect(screen.getByText(/No providers configured yet/)).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: 'Add Provider' }))

    expect(screen.getByText('Models')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Add Model' })).toBeInTheDocument()
  })

  it('add model entry: form shows new model name and display name inputs', async () => {
    const user = userEvent.setup()
    mocks.listProviders.mockResolvedValue({ providers: [] })

    render(Providers)
    await waitFor(() => expect(screen.getByText(/No providers configured yet/)).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: 'Add Provider' }))
    await user.click(screen.getByRole('button', { name: 'Add Model' }))

    expect(screen.getByPlaceholderText('e.g. gpt-4.1')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('e.g. GPT 4.1')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Remove model' })).toBeInTheDocument()
  })

  it('shows Summarization checkbox and hint for each model', async () => {
    const user = userEvent.setup()
    mocks.listProviders.mockResolvedValue({ providers: [] })

    render(Providers)
    await waitFor(() => expect(screen.getByText(/No providers configured yet/)).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: 'Add Provider' }))
    await user.click(screen.getByRole('button', { name: 'Add Model' }))
    await user.click(screen.getByRole('button', { name: 'Add Model' }))

    const boxes = screen.getAllByRole('checkbox', { name: 'Summarization' })
    expect(boxes).toHaveLength(2)
    expect(
      screen.getAllByText(
        /Use this model for summarization tasks \(e\.g\. session titles\)\. Prefer fast, inexpensive models\./,
      ),
    ).toHaveLength(2)
  })

  it('submit create with models calls API with models payload', async () => {
    const user = userEvent.setup()
    const model = makeModelConfig()
    const newProvider = makeProvider({
      displayName: undefined,
      models: [model],
    })
    mocks.listProviders
      .mockResolvedValueOnce({ providers: [] })
      .mockResolvedValueOnce({ providers: [newProvider] })
    mocks.createProvider.mockResolvedValue(newProvider)

    render(Providers)
    await waitFor(() => expect(screen.getByText(/No providers configured yet/)).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: 'Add Provider' }))
    await user.type(screen.getByLabelText('Name'), newProvider.name)
    await user.type(screen.getByLabelText('Base URL'), newProvider.baseUrl)
    await user.type(screen.getByLabelText('API Key'), faker.string.alphanumeric(32))

    await user.click(screen.getByRole('button', { name: 'Add Model' }))
    await user.type(screen.getByPlaceholderText('e.g. gpt-4.1'), model.name)
    await user.type(screen.getByPlaceholderText('e.g. GPT 4.1'), model.displayName!)

    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(mocks.createProvider).toHaveBeenCalledOnce()
      expect(mocks.createProvider).toHaveBeenCalledWith(
        expect.objectContaining({
          body: expect.objectContaining({
            models: [model],
          }),
        }),
      )
    })
  })

  it('create flow sends summarization true when Summarization is checked', async () => {
    const user = userEvent.setup()
    const model = makeModelConfig({ summarization: true })
    const newProvider = makeProvider({
      displayName: undefined,
      models: [model],
    })
    mocks.listProviders
      .mockResolvedValueOnce({ providers: [] })
      .mockResolvedValueOnce({ providers: [newProvider] })
    mocks.createProvider.mockResolvedValue(newProvider)

    render(Providers)
    await waitFor(() => expect(screen.getByText(/No providers configured yet/)).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: 'Add Provider' }))
    await user.type(screen.getByLabelText('Name'), newProvider.name)
    await user.type(screen.getByLabelText('Base URL'), newProvider.baseUrl)
    await user.type(screen.getByLabelText('API Key'), faker.string.alphanumeric(32))

    await user.click(screen.getByRole('button', { name: 'Add Model' }))
    await user.type(screen.getByPlaceholderText('e.g. gpt-4.1'), model.name)
    await user.type(screen.getByPlaceholderText('e.g. GPT 4.1'), model.displayName!)
    await user.click(screen.getByRole('checkbox', { name: 'Summarization' }))

    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(mocks.createProvider).toHaveBeenCalledWith(
        expect.objectContaining({
          body: expect.objectContaining({
            models: [model],
          }),
        }),
      )
    })
  })

  it('edit provider form shows existing models', async () => {
    const user = userEvent.setup()
    const m1 = makeModelConfig({ summarization: false })
    const m2 = {
      name: `${faker.word.noun()}-${faker.string.alphanumeric(4)}`,
      displayName: undefined,
      summarization: false as const,
    }
    const provider = makeProvider({
      models: [m1, m2],
    })
    mocks.listProviders.mockResolvedValue({ providers: [provider] })

    render(Providers)
    await waitFor(() => expect(screen.getByText(provider.displayName!)).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: 'Edit' }))

    const modelInputs = screen.getAllByPlaceholderText('e.g. gpt-4.1')
    expect(modelInputs).toHaveLength(2)
    expect(modelInputs[0]).toHaveValue(m1.name)
    expect(modelInputs[1]).toHaveValue(m2.name)
    const displayNameInputs = screen.getAllByPlaceholderText('e.g. GPT 4.1')
    expect(displayNameInputs[0]).toHaveValue(m1.displayName!)
  })

  it('provider list shows model count', async () => {
    const provider = makeProvider({
      models: [makeModelConfig(), { ...makeModelConfig(), displayName: undefined }],
    })
    mocks.listProviders.mockResolvedValue({ providers: [provider] })

    render(Providers)
    await waitFor(() => {
      expect(screen.getByText('2 models')).toBeInTheDocument()
    })
  })
})
