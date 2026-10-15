<script lang="ts">
	import { getI18n } from '../../i18n/i18n.svelte';
	// Badge status KYC gaya DOKU. `recheck` = NEEDS_REVIEW_AGAIN (approved + perlu tinjau ulang).
	let { status, recheck = false, size = 'md' }: { status: string; recheck?: boolean; size?: 'sm' | 'md' } = $props();
	const { t, tx } = getI18n();

	// Pemetaan status Go -> tampilan. Sumber status tunggal: merchants.onboarding_status.
	const META: Record<string, { cls: string; dot: string }> = {
		draft: { cls: 'bg-[#EDF0F3] text-[#42505C]', dot: 'bg-[#7A8894]' },
		submitted: { cls: 'bg-[#FFF4E0] text-[#8A5A00]', dot: 'bg-[#E8B84A]' },
		under_review: { cls: 'bg-[#E7F0FB] text-[#1D4F91]', dot: 'bg-[#3B7DD8]' },
		action_required: { cls: 'bg-[#FDECEC] text-[#B02424]', dot: 'bg-[#D32F2F]' },
		approved: { cls: 'bg-[#E4F5EC] text-[#1D7A46]', dot: 'bg-[#1D7A46]' },
		rejected: { cls: 'bg-[#FDECEC] text-[#B02424]', dot: 'bg-[#D32F2F]' },
		suspended: { cls: 'bg-[#EDE8F7] text-[#5B3FA8]', dot: 'bg-[#5B3FA8]' }
	};
	const m = $derived(
		META[status] ?? { cls: 'bg-[#EDF0F3] text-[#42505C]', dot: 'bg-[#7A8894]' }
	);
	const shown = $derived(recheck ? t('kyc.st.recheck') : tx('kyc.st.' + status, status));
</script>

<span
	class="inline-flex items-center gap-1.5 rounded-full font-semibold tracking-wide
		{recheck ? 'bg-[#FFF4E0] text-[#8A5A00]' : m.cls}
		{size === 'sm' ? 'px-2.5 py-0.5 text-[11px]' : 'px-3 py-1 text-[12px]'}"
>
	<span class="size-1.5 rounded-full {recheck ? 'bg-[#E8B84A]' : m.dot}"></span>
	{shown}
</span>
