<script lang="ts">
  import CreditCard from '@lucide/svelte/icons/credit-card'
  import KeyRound from '@lucide/svelte/icons/key-round'
  import Paperclip from '@lucide/svelte/icons/paperclip'
  import StickyNote from '@lucide/svelte/icons/sticky-note'

  // Иконка типа записи.
  let { type, size = 16 }: { type: string; size?: number } = $props()

  const icons = { credentials: KeyRound, text: StickyNote, card: CreditCard, binary: Paperclip }
  let Icon = $derived(icons[type as keyof typeof icons] ?? StickyNote)
</script>

<Icon {size} aria-hidden="true" />
