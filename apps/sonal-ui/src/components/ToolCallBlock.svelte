<script lang="ts">
  interface Props {
    name: string
    args: Record<string, unknown>
    response?: Record<string, unknown>
  }

  let { name, args, response }: Props = $props()
</script>

<details class="tool-call-block">
  <summary class="tool-call-summary">
    <span class="tool-call-label">Tool call</span>
    <span class="tool-call-name">{name}</span>
  </summary>
  <div class="tool-call-body">
    <div class="tool-call-section">
      <div class="tool-call-section-title">Arguments</div>
      <pre class="tool-call-pre">{JSON.stringify(args, null, 2)}</pre>
    </div>
    {#if response !== undefined}
      <div class="tool-call-section">
        <div class="tool-call-section-title">Response</div>
        <pre class="tool-call-pre">{JSON.stringify(response, null, 2)}</pre>
      </div>
    {/if}
  </div>
</details>

<style>
  .tool-call-block {
    border: 1px solid var(--border);
    border-radius: var(--radius-default);
    background: var(--surface-raised);
    font-size: var(--font-size-caption);
    max-width: 100%;
    box-sizing: border-box;
  }

  .tool-call-summary {
    display: flex;
    align-items: center;
    gap: var(--space-8);
    padding: var(--space-8) var(--space-12);
    cursor: pointer;
    user-select: none;
    list-style: none;
  }

  .tool-call-summary::-webkit-details-marker {
    display: none;
  }

  .tool-call-summary::before {
    content: '▶';
    font-size: 10px;
    color: var(--text);
    transition: transform 120ms ease;
    flex-shrink: 0;
  }

  details[open] > .tool-call-summary::before {
    transform: rotate(90deg);
  }

  .tool-call-label {
    font-size: 11px;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-primary-light);
    background: var(--color-primary-dark);
    padding: 2px var(--space-8);
    border-radius: var(--radius-default);
    border: 1px solid var(--border);
    flex-shrink: 0;
  }

  .tool-call-name {
    font-family: var(--mono);
    font-size: 13px;
    font-weight: 500;
    color: var(--text-on-raised);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .tool-call-body {
    padding: var(--space-8) var(--space-12) var(--space-12);
    border-top: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: var(--space-12);
  }

  .tool-call-section-title {
    font-size: 11px;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text);
    margin-bottom: var(--space-4);
  }

  .tool-call-pre {
    margin: 0;
    font-family: var(--mono);
    font-size: 12px;
    line-height: 1.5;
    color: var(--text-on-raised);
    overflow-x: auto;
    white-space: pre;
    background: transparent;
    padding: 0;
  }
</style>
