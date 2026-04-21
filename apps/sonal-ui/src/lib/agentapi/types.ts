/**
 * Re-exports OpenAPI-aligned types from generated code (`agentapi.generated.ts`).
 * Single source of truth: `runtime/internal/agentapi/openapi.yaml` → `make generate-api`.
 */

import type { components } from './agentapi.generated'

export type AgentRunRequest = components['schemas']['AgentRunRequest']
export type UserMessageContent = components['schemas']['UserMessageContent']
export type UserMessagePart = components['schemas']['UserMessagePart']

export type AgentStreamPart = components['schemas']['AgentStreamPart']
export type AgentStreamContent = components['schemas']['AgentStreamContent']
export type ToolCallData = components['schemas']['ToolCallData']
export type ToolResultData = components['schemas']['ToolResultData']

export type SessionBoundEvent = components['schemas']['SessionBoundEvent']
export type SessionStatusEvent = components['schemas']['SessionStatusEvent']
export type AgentStreamEvent = components['schemas']['AgentStreamEvent']
export type StreamErrorEvent = components['schemas']['StreamErrorEvent']
export type DoneEvent = components['schemas']['DoneEvent']
export type StreamEvent = components['schemas']['StreamEvent']

export type ProviderResponse = components['schemas']['ProviderResponse']
export type ProviderListResponse = components['schemas']['ProviderListResponse']
export type CreateProviderRequest = components['schemas']['CreateProviderRequest']
export type UpdateProviderRequest = components['schemas']['UpdateProviderRequest']
export type ModelConfig = components['schemas']['ModelConfig']
export type ModelInfo = components['schemas']['ModelInfo']
export type ModelListResponse = components['schemas']['ModelListResponse']

export type SessionMetadata = components['schemas']['SessionMetadata']
export type SessionListResponse = components['schemas']['SessionListResponse']

/** Runtime discriminator check for SSE JSON payloads (agent stream contract). */
export function isStreamEvent(value: unknown): value is StreamEvent {
  if (typeof value !== 'object' || value === null || !('event' in value)) {
    return false
  }
  const e = (value as { event: unknown }).event
  return e === 'sessionBound' || e === 'sessionStatus' || e === 'agent' || e === 'error' || e === 'done'
}
