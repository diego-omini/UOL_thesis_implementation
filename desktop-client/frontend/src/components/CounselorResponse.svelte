<script lang="ts">
  // CounselorResponse.svelte safely renders counselor Markdown and opens citations outside the application.
  import DOMPurify from 'dompurify'
  import { marked } from 'marked'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
  import type { AIResponseView } from '../lib/types'

  export let response: AIResponseView
  export let currentContextAsOf = ''

  function renderMarkdown(value: string): string {
    const rendered = marked.parse(value, { async: false, gfm: true })
    const sanitized = DOMPurify.sanitize(rendered, {
      ALLOWED_TAGS: ['p', 'br', 'strong', 'em', 'ul', 'ol', 'li', 'blockquote', 'code', 'pre', 'a', 'h2', 'h3', 'table', 'thead', 'tbody', 'tr', 'th', 'td'],
      ALLOWED_ATTR: ['href', 'title'],
    })
    const template = document.createElement('template')
    template.innerHTML = sanitized
    for (const anchor of template.content.querySelectorAll('a')) {
      const raw = anchor.getAttribute('href')
      try {
        const url = raw ? new URL(raw) : null
        if (!url || (url.protocol !== 'https:' && url.protocol !== 'http:')) throw new Error('unsupported URL')
        anchor.dataset.externalUrl = url.toString()
        anchor.removeAttribute('href')
        anchor.setAttribute('role', 'link')
        anchor.setAttribute('tabindex', '0')
      } catch {
        anchor.removeAttribute('href')
      }
    }
    return template.innerHTML
  }

  function label(value: string): string {
    return value.replaceAll('_', ' ')
  }

  function openCitation(event: MouseEvent | KeyboardEvent): void {
    if (event instanceof KeyboardEvent && event.key !== 'Enter' && event.key !== ' ') return
    const target = event.target
    if (!(target instanceof Element)) return
    const anchor = target.closest('[data-external-url]')
    if (!(anchor instanceof HTMLElement)) return

    event.preventDefault()
    try {
      const href = anchor.dataset.externalUrl
      if (!href) return
      const url = new URL(href)
      if (url.protocol === 'https:' || url.protocol === 'http:') {
        BrowserOpenURL(url.toString())
      }
    } catch {
      // Invalid provider links remain inert.
    }
  }

  function provenanceState(): string {
    if (!response.ContextAsOf) return 'Source-data timestamp was not recorded for this saved result.'
    if (!currentContextAsOf) return `Data calculated ${new Date(response.ContextAsOf).toLocaleString()}.`
    if (response.ContextAsOf === currentContextAsOf) return `Uses the currently displayed data from ${new Date(response.ContextAsOf).toLocaleString()}.`
    return `Uses data from ${new Date(response.ContextAsOf).toLocaleString()}; the currently displayed figures have a different timestamp.`
  }
</script>

<div class="ai-response" aria-live="polite">
  <div class="ai-response-content" on:click={openCitation} on:keydown={openCitation}>
    {#if response.HasStructured}
      <section class="summary">
        <h2>Summary</h2>
        <div>{@html renderMarkdown(response.Structured.Summary)}</div>
      </section>

      <section>
        <h2>Observations</h2>
        <div class="observations">
          {#each response.Structured.Observations as observation}
            <article class="observation">
              <div class="observation-meta">
                <span>{label(observation.Category)}</span>
                <span>{label(observation.Severity)} relevance</span>
                <span>{label(observation.Confidence)} confidence</span>
              </div>
              <h3>{observation.Statement}</h3>
              <div>{@html renderMarkdown(observation.Explanation)}</div>

              {#if observation.ReportedValues.length > 0}
                <dl class="reported-values">
                  {#each observation.ReportedValues as value}
                    <div>
                      <dt>{value.FactID}</dt>
                      <dd>{value.Value} {value.Unit}</dd>
                    </div>
                  {/each}
                </dl>
              {/if}

              {#if observation.SupportingFactIDs.length > 0}
                <p class="support"><strong>Supporting facts:</strong> {observation.SupportingFactIDs.join(', ')}</p>
              {/if}
              {#if observation.Limitations.length > 0}
                <ul class="observation-limitations">
                  {#each observation.Limitations as limitation}<li>{limitation}</li>{/each}
                </ul>
              {/if}
              {#if observation.Citations.length > 0}
                <p class="citations">
                  <strong>Sources:</strong>
                  {#each observation.Citations as citation, index}
                    <button type="button" class="citation-link" data-external-url={citation.URL}>{citation.Title || citation.URL}</button>{index < observation.Citations.length - 1 ? ', ' : ''}
                  {/each}
                </p>
              {/if}
            </article>
          {/each}
        </div>
      </section>

      <section class="response-limitations">
        <h2>Limitations</h2>
        <ul>
          {#each response.Structured.Limitations as limitation}
            <li>
              {limitation.Statement}
              {#if limitation.FactIDs.length > 0}<small>{limitation.FactIDs.join(', ')}</small>{/if}
            </li>
          {/each}
        </ul>
      </section>
    {:else}
      {@html renderMarkdown(response.Text)}
    {/if}
  </div>
  <footer>
    <small>Created {response.CreatedAt ? new Date(response.CreatedAt).toLocaleString() : 'at an unrecorded time'} · {response.Provider} · {response.Model} · prompt v{response.PromptVersion}</small>
    <small class:mismatch={Boolean(currentContextAsOf && response.ContextAsOf && currentContextAsOf !== response.ContextAsOf)}>{provenanceState()}</small>
  </footer>
</div>

<style>
  .ai-response {
    display: grid;
    gap: 10px;
    margin-top: 16px;
    padding: 14px 16px;
    border-left: 4px solid #1f6f8b;
    background: #f5f9fb;
    line-height: 1.55;
  }

  .ai-response small {
    color: #5d6a7a;
  }

  footer { display: grid; gap: 3px; padding-top: 10px; border-top: 1px solid #d8e0e8; }
  footer .mismatch { color: #9a5b18; font-weight: 700; }

  .ai-response-content :global(h2) {
    margin: 18px 0 6px;
    color: #243444;
    font-size: 18px;
    line-height: 1.35;
  }

  .observations {
    display: grid;
    gap: 12px;
  }

  .observation {
    padding: 12px 14px;
    border: 1px solid #d8e0e8;
    border-radius: 6px;
    background: white;
  }

  .observation-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    color: #42576a;
    font-size: 12px;
    text-transform: capitalize;
  }

  .observation-meta span {
    padding: 2px 7px;
    border-radius: 999px;
    background: #e8f0f4;
  }

  .reported-values {
    display: grid;
    gap: 4px;
    margin: 10px 0;
  }

  .reported-values div {
    display: grid;
    grid-template-columns: minmax(180px, 1fr) auto;
    gap: 12px;
  }

  .reported-values dt,
  .reported-values dd {
    margin: 0;
  }

  .reported-values dt,
  .support,
  .response-limitations small {
    color: #5d6a7a;
    font-family: Consolas, monospace;
    font-size: 12px;
    overflow-wrap: anywhere;
  }

  .response-limitations small {
    display: block;
    margin-top: 2px;
  }

  .ai-response-content :global(h2:first-child),
  .ai-response-content :global(p:first-child) {
    margin-top: 0;
  }

  .ai-response-content :global(h3) {
    margin: 14px 0 6px;
    color: #32485c;
    font-size: 16px;
  }

  .ai-response-content :global(p) {
    margin: 8px 0;
  }

  .ai-response-content :global(ul),
  .ai-response-content :global(ol) {
    margin: 8px 0;
    padding-left: 24px;
  }

  .ai-response-content :global(li + li) {
    margin-top: 5px;
  }

  .ai-response-content :global(a) {
    color: #14637d;
    overflow-wrap: anywhere;
  }

  .citation-link {
    padding: 0;
    border: 0;
    background: transparent;
    color: #14637d;
    font: inherit;
    text-decoration: underline;
    cursor: pointer;
    overflow-wrap: anywhere;
  }

  .ai-response-content :global(code) {
    padding: 1px 4px;
    border-radius: 3px;
    background: #e8f0f4;
    font-family: Consolas, monospace;
  }

  .ai-response-content :global(table) {
    width: 100%;
    margin: 12px 0;
    border-collapse: collapse;
    background: white;
    font-size: 14px;
  }

  .ai-response-content :global(th),
  .ai-response-content :global(td) {
    padding: 8px 10px;
    border: 1px solid #d8e0e8;
    text-align: left;
    vertical-align: top;
  }

  .ai-response-content :global(th) {
    background: #e8f0f4;
    color: #243444;
  }
</style>
