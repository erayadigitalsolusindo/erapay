<script lang="ts">
	import { getI18n } from '../i18n/i18n.svelte';
	import type { Key } from '../i18n/messages';
	// Status selalu berlabel teks, bukan warna saja (PRD §4.11). `kind` memilih kamus label.
	let { kind, value }: { kind: 'po' | 'rf' | 'bank' | 'wh' | 'wd' | 'settle' | 'onb'; value: string } = $props();
	const { t } = getI18n();
	const key = $derived(
		(kind === 'onb' ? `onb.st.${value}` : kind === 'bank' ? `bank.st.${value}` : kind === 'wh' ? `wh.st.${value}` : kind === 'wd' ? `wh.d.${value}` : kind === 'settle' ? `settle.${value}` : `${kind}.st.${value}`) as Key
	);
	const good = ['paid', 'refunded', 'verified', 'succeeded', 'settled', 'active'];
	const wait = ['pending_approval', 'approved', 'pending_verification', 'pending', 'expected', 'submitted', 'under_review', 'action_required'];
	const bad = ['failed', 'dead'];
	// Untuk onboarding, 'approved' berarti aktif (hijau), bukan "menunggu" seperti pada refund.
	const tone = $derived(
		kind === 'onb' && value === 'approved' ? 'bg-emerald-100 text-emerald-800' : kind === 'onb' && value === 'rejected' ? 'bg-rose-100 text-rose-800' : good.includes(value) ? 'bg-emerald-100 text-emerald-800' : wait.includes(value) ? 'bg-amber-100 text-amber-800' : bad.includes(value) ? 'bg-rose-100 text-rose-800' : 'bg-ink/10 text-ink'
	);
</script>

<span class="pill {tone}">{t(key)}</span>
