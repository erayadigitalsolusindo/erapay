<script lang="ts">
	// Catatan reviewer per bagian/field (NEEDS_REVISION). Ditempel di atas kartu bagian terkait.
	import type { Note } from '../../server/kyc';

	let { notes, section }: { notes: Note[]; section?: string } = $props();
	const mine = $derived(section ? notes.filter((n) => !n.resolved && n.section === section) : notes);
</script>

{#if mine.length > 0}
	<div class="rounded-md border border-[#F3C1C1] bg-[#FDF4F4] p-3">
		<p class="mb-1.5 flex items-center gap-1.5 text-[12px] font-bold uppercase tracking-wide text-[#D32F2F]">
			<svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2">
				<path d="M12 9v4" /><path d="M12 17h.01" /><circle cx="12" cy="12" r="9" />
			</svg>
			Catatan reviewer — perlu diperbaiki
		</p>
		<ul class="space-y-1.5">
			{#each mine as n (n.id)}
				<li class="text-[13px] leading-relaxed text-[#6B3A3A]">
					{#if n.field}<span class="font-semibold text-[#B02424]">{n.field}: </span>{/if}{n.note}
				</li>
			{/each}
		</ul>
	</div>
{/if}
