<script lang="ts">
	import { onMount } from 'svelte';
	import { terbilang } from '../../../lib/utils/terbilang';

	const KEY = 'erapay.suara';
	let on = $state(false);
	const seen = new Set<string>();
	let primed = false;

	const bicara = (teks: string) => {
		const u = new SpeechSynthesisUtterance(teks);
		u.lang = 'id-ID';
		speechSynthesis.speak(u); // antrean bawaan: beberapa pembayaran dibacakan bergantian
	};

	async function poll() {
		try {
			const r = await fetch('/dashboard/suara');
			if (!r.ok) return;
			const rows: { id: string; amount: number }[] = await r.json();
			const baru = rows.filter((p) => !seen.has(p.id));
			baru.forEach((p) => seen.add(p.id));
			// Putaran pertama hanya mencatat yang sudah ada, supaya pembayaran lama tidak dibacakan.
			if (primed) baru.reverse().forEach((p) => bicara(`Transaksi dengan nominal ${terbilang(p.amount)} rupiah berhasil. Terima kasih.`));
			primed = true;
		} catch {
			/* jaringan putus: coba lagi di putaran berikutnya */
		}
	}

	onMount(() => {
		try {
			on = localStorage.getItem(KEY) === '1' && 'speechSynthesis' in window;
		} catch {
			/* penyimpanan diblokir: default mati */
		}
	});

	$effect(() => {
		if (!on) return;
		primed = false;
		poll();
		const t = setInterval(poll, 4000);
		return () => clearInterval(t);
	});

	function toggle() {
		on = !on;
		try {
			localStorage.setItem(KEY, on ? '1' : '0');
		} catch {
			/* abaikan */
		}
		// Ketukan ini juga yang membuka izin suara di browser.
		if (on) bicara('Suara notifikasi pembayaran aktif.');
		else speechSynthesis.cancel();
	}
</script>

{#if typeof window === 'undefined' || 'speechSynthesis' in window}
	<button
		type="button"
		onclick={toggle}
		aria-pressed={on}
		class="fixed bottom-4 right-4 z-40 rounded-full px-4 py-2 text-xs font-bold shadow-lg transition {on ? 'bg-merah text-white' : 'bg-surface text-ink ring-1 ring-ink/20'}"
	>
		{on ? '🔊 Suara aktif' : '🔇 Suara mati'}
	</button>
{/if}
