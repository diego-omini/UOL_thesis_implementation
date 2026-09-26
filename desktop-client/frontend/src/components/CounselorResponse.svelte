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
    {@html renderMarkdown(response.Text)}
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
