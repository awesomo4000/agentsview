<script lang="ts">
  import { onDestroy } from "svelte";
  import { Button, Chip, IconButton, TextInput } from "@kenn-io/kit-ui";
  import { SessionsService } from "../../api/generated/index.js";
  import { XIcon } from "../../icons.js";
  import { settings } from "../../stores/settings.svelte.js";
  import { sessions } from "../../stores/sessions.svelte.js";
  import { m } from "../../i18n/index.js";

  let { sessionId, labels, onFilterLabel }: {
    sessionId: string;
    labels: string[];
    onFilterLabel?: (label: string) => void;
  } = $props();
  let currentLabels = $state<string[]>([]);
  let tag = $state("");
  let summary = $state("");
  let savedSummary = $state("");
  let loading = $state(true);
  let loadFailed = $state(false);
  let failed = $state(false);
  let saved = $state(false);
  let busy = $state(false);
  let disposed = false;
  const read = new AbortController();
  $effect(() => { currentLabels = [...labels]; });
  onDestroy(() => { disposed = true; read.abort(); });

  $effect(() => {
    const id = sessionId;
    void SessionsService.getApiV1SessionsByIdSummary({ id }, { signal: read.signal })
      .then((value) => { if (!disposed) { summary = value.content; savedSummary = value.content; } })
      .catch(() => { if (!disposed) loadFailed = true; })
      .finally(() => { if (!disposed) loading = false; });
  });

  async function changeTag(remove?: string) {
    if (busy || settings.readOnly || (!remove && !tag.trim())) return;
    busy = true; failed = false; saved = false;
    try {
      const result = await SessionsService.patchApiV1SessionsByIdLabels(
        { id: sessionId }, remove ? { remove: [remove] } : { add: [tag.trim()] },
      );
      if (disposed) return;
      currentLabels = result.labels;
      if (!remove) tag = "";
      void sessions.refreshActiveSession();
      sessions.refreshSidebarIfAttached();
    } catch { if (!disposed) failed = true; }
    finally { if (!disposed) busy = false; }
  }

  async function saveSummary() {
    if (busy || loading || loadFailed || settings.readOnly) return;
    busy = true; failed = false; saved = false;
    try {
      const result = await SessionsService.putApiV1SessionsByIdSummary({ id: sessionId }, { content: summary });
      if (disposed) return;
      summary = result.content; savedSummary = result.content; saved = true;
    } catch { if (!disposed) failed = true; }
    finally { if (!disposed) busy = false; }
  }
</script>

<section class="annotations" aria-label={m.session_annotations_title()}>
  <div class="label">{m.session_vitals_labels()}</div>
  <div class="tags">
    {#each currentLabels as label (label)}
      <div class="tag">
        <Chip size="xs" tone="neutral" uppercase={false} interactive={!!onFilterLabel}
          onclick={() => onFilterLabel?.(label)}>{label}</Chip>
        {#if !settings.readOnly}
          <IconButton size="sm" ariaLabel={m.session_annotations_remove({ tag: label })}
            disabled={busy} onclick={() => changeTag(label)}><XIcon size={12} aria-hidden="true" /></IconButton>
        {/if}
      </div>
    {/each}
  </div>
  {#if !settings.readOnly}
    <div class="add-tag">
      <TextInput bind:value={tag} ariaLabel={m.session_annotations_tag()} placeholder={m.session_annotations_tag()}
        disabled={busy} onkeydown={(event) => { if (event.key === "Enter") { event.preventDefault(); void changeTag(); } }} />
      <Button size="sm" label={m.session_annotations_add()} disabled={busy || !tag.trim()} onclick={() => changeTag()} />
    </div>
  {/if}
  <label class="label" for="session-summary">{m.session_annotations_summary()}</label>
  {#if loadFailed}
    <p role="alert">{m.session_annotations_load_failed()}</p>
  {:else}
    <textarea id="session-summary" bind:value={summary} rows="5" maxlength="16000"
      disabled={loading || busy || settings.readOnly} oninput={() => { saved = false; }}></textarea>
    {#if !settings.readOnly}
      <Button size="sm" label={m.session_annotations_save()} disabled={loading || busy || summary === savedSummary} onclick={saveSummary} />
    {/if}
  {/if}
  {#if failed}<p role="alert">{m.session_annotations_save_failed()}</p>{/if}
  {#if saved}<p role="status">{m.session_annotations_saved()}</p>{/if}
</section>

<style>
  .annotations { display: grid; gap: 8px; padding-block: 12px; border-top: 1px solid var(--border-muted); }
  .label { font-size: 11px; color: var(--text-muted); }
  .tags, .tag, .add-tag { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
  textarea { box-sizing: border-box; width: 100%; resize: vertical; padding: 8px; border: 1px solid var(--border-muted); border-radius: 4px; background: var(--bg-surface); color: var(--text-primary); font: inherit; font-size: 12px; }
  p { margin: 0; font-size: 12px; }
</style>
