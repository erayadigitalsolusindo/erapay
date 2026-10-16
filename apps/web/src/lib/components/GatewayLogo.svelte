<script lang="ts">
	// Logo gateway. Pakai berkas resmi di static/gateways/<kode>.svg (atau .png) bila ada; kalau belum ada,
	// tampil lencana teks supaya tidak ada gambar rusak. Letakkan logo resmi dari brand kit gateway, jangan digambar ulang.
	let { code, label, class: cls = 'size-12' }: { code: string; label: string; class?: string } = $props();
	const SRC = $derived([`/gateways/${code}.svg`, `/gateways/${code}.png`]);
	let idx = $state(0);
	const TONE: Record<string, string> = { doku: 'from-[#ed1c24] to-[#b5121b]' };
</script>

{#if idx < SRC.length}
	<img src={SRC[idx]} alt={label} class="w-auto shrink-0 object-contain {cls}" onerror={() => idx++} />
{:else}
	<span class="grid shrink-0 place-items-center rounded-2xl bg-linear-to-br {TONE[code] ?? 'from-slate-500 to-slate-700'} px-3 font-display text-[11px] font-extrabold tracking-wide text-white uppercase shadow-sm {cls}" role="img" aria-label={label}>
		{label}
	</span>
{/if}
