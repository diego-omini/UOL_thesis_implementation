<script lang="ts">
  // CounselorResponse.svelte safely renders counselor Markdown and opens citations outside the application.
  import DOMPurify from 'dompurify'
  import { marked } from 'marked'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
  import type { AIResponseView } from '../lib/types'

  export let response: AIResponseView

  function renderMarkdown(value: string): string {
    const rendered = marked.parse(value, { async: false, gfm: true })
    return DOMPurify.sanitize(rendered)
  }

  function label(value: string): string {
    return value.replaceAll('_', ' ')
  }

  function openCitation(event: MouseEvent | KeyboardEvent): void {
    if (event instanceof KeyboardEvent && event.key !== 'Enter' && event.key !== ' ') return
    const target = event.target
    if (!(target instanceof Element)) return
    const anchor = target.closest('a')
    if (!(anchor instanceof HTMLAnchorElement)) return

    event.preventDefault()
    try {
      const href = anchor.getAttribute('href')
      if (!href) return
      const url = new URL(href)
      if (url.protocol === 'https:' || url.protocol === 'http:') {
        BrowserOpenURL(url.toString())
      }
    } catch {
      // Invalid provider links remain inert.
    }
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
                    <a href={citation.URL}>{citation.Title || citation.URL}</a>{index < observation.Citations.length - 1 ? ', ' : ''}
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
  <small>{response.Provider} · {response.Model} · {response.PromptID} v{response.PromptVersion}</small>
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
