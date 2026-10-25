<script lang="ts">
  import { Badge } from '#lib/ui/index.js';

  // The unread count pill, shared by the messaging dock and the conversation rows.
  let { count }: { count: number } = $props();
</script>

{#if count}<Badge variant="brand" class="shrink-0 rounded-full bg-brand py-0 font-normal text-brand-foreground">{count}<span class="sr-only"> unread</span></Badge>{/if}
