<script lang="ts">
  // Short guides to what matters most: a list of topics and the steps of the
  // open one. The text lives in strings.ts; parts between backticks show as
  // keys or Markdown, never as HTML.
  import Icon from './Icon.svelte'
  import { router } from '../lib/router.svelte'
  import { strings } from '../lib/strings'

  const t = strings.tutorials
  const topic = $derived(t.topics.find((x) => x.id === router.topic) ?? t.topics[0]!)
  const index = $derived(t.topics.indexOf(topic))
  const next = $derived(t.topics[index + 1] ?? null)

  /** A step split into text and `code` parts. */
  const parts = (text: string) => text.split('`').map((p, i) => ({ code: i % 2 === 1, text: p }))
</script>

{#snippet rich(text: string)}{#each parts(text) as p, i (i)}{#if p.code}<kbd>{p.text}</kbd>{:else}{p.text}{/if}{/each}{/snippet}

<section class="tutorials" aria-labelledby="tutorials-title">
  <header>
    <h1 id="tutorials-title">{t.title}</h1>
    <p class="intro">{t.intro}</p>
  </header>
  <div class="body">
    <nav aria-label={t.topicsLabel}>
      <ol>
        {#each t.topics as x, i (x.id)}
          <li>
            <button type="button" aria-current={x.id === topic.id ? 'page' : undefined} onclick={() => router.goTutorials(x.id)}>
              <span class="num">{i + 1}</span>
              <span class="what">
                <strong>{x.title}</strong>
                <span>{x.summary}</span>
              </span>
            </button>
          </li>
        {/each}
      </ol>
    </nav>
    <article aria-labelledby="topic-title">
      <h2 id="topic-title">{topic.title}</h2>
      <p class="summary">{topic.summary}</p>
      <ol class="steps">
        {#each topic.steps as step, i (i)}
          <li>
            <span class="step-num" aria-label={t.stepOf(i + 1, topic.steps.length)}>{i + 1}</span>
            <p>{@render rich(step)}</p>
          </li>
        {/each}
      </ol>
      {#if 'tip' in topic && topic.tip}
        <aside class="tip"><strong>{t.tip}:</strong> {@render rich(topic.tip)}</aside>
      {/if}
      {#if next}
        <button type="button" class="next" onclick={() => router.goTutorials(next.id)}>
          {next.title}
          <Icon name="chevron" size={16} />
        </button>
      {/if}
    </article>
  </div>
</section>

<style>
  .tutorials {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
    overflow: auto;
  }
  header {
    padding: var(--space-6) var(--space-6) var(--space-4);
    border-bottom: 1px solid var(--line);
  }
  h1 {
    margin: 0 0 var(--space-1);
    font-size: var(--text-xl);
    font-weight: 650;
  }
  .intro,
  .summary {
    color: var(--text-muted);
  }
  .body {
    display: grid;
    grid-template-columns: minmax(220px, 300px) minmax(0, 1fr);
    gap: var(--space-6);
    padding: var(--space-6);
  }
  nav ol {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  nav button {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    width: 100%;
    padding: 10px 12px;
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text);
    text-align: left;
    cursor: pointer;
  }
  nav button:hover {
    background: var(--bg-hover);
  }
  nav button[aria-current='page'] {
    border-color: var(--line);
    background: var(--accent-soft);
  }
  .num,
  .step-num {
    display: grid;
    flex: none;
    place-items: center;
    width: 24px;
    height: 24px;
    border-radius: 50%;
    background: var(--bg-sunken);
    color: var(--text-muted);
    font-size: var(--text-xs);
    font-weight: 650;
  }
  nav button[aria-current='page'] .num {
    background: var(--accent);
    color: var(--on-accent);
  }
  .what {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
    font-size: var(--text-sm);
  }
  .what span {
    color: var(--text-muted);
    font-size: var(--text-xs);
  }
  article {
    max-width: 680px;
  }
  h2 {
    margin: 0 0 var(--space-1);
    font-size: var(--text-lg);
    font-weight: 650;
  }
  .steps {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    margin: var(--space-4) 0;
    padding: 0;
    list-style: none;
  }
  .steps li {
    display: flex;
    align-items: flex-start;
    gap: 12px;
  }
  .steps p {
    margin: 2px 0 0;
    line-height: 1.6;
  }
  kbd {
    padding: 1px 6px;
    border: 1px solid var(--line-strong);
    border-radius: 5px;
    background: var(--bg-raised);
    font-family: var(--font-mono);
    font-size: 0.88em;
    white-space: nowrap;
  }
  .tip {
    padding: 12px 14px;
    border-left: 3px solid var(--accent);
    border-radius: var(--radius-sm);
    background: var(--accent-soft);
    font-size: var(--text-sm);
  }
  .next {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    margin-top: var(--space-4);
    padding: 8px 12px;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text);
    font-weight: 550;
    cursor: pointer;
  }
  .next:hover {
    background: var(--bg-hover);
  }
  @media (max-width: 760px) {
    .body {
      grid-template-columns: 1fr;
    }
  }
</style>
