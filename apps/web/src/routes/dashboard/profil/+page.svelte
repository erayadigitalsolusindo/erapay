<script lang="ts">
	import { enhance } from '$app/forms';
	import { fade, fly, scale } from 'svelte/transition';
	import { flip } from 'svelte/animate';
	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	import { notify } from '../../../lib/ui/dialog.svelte';
	import Modal from '../../../lib/components/Modal.svelte';

	let { data } = $props();
	const i18n = getI18n();
	const { t } = i18n;

	const profile = $derived(data.profile);
	type Tab = 'profile' | 'password' | 'mfa';
	let tab = $state<Tab>('profile');
	let dir = $state(1);
	const TABS: Tab[] = ['profile', 'password', 'mfa'];
	function go(next: Tab) {
		dir = TABS.indexOf(next) >= TABS.indexOf(tab) ? 1 : -1;
		tab = next;
	}

	// Animasi dihormati sistem "kurangi gerakan": durasi 0 bila diminta.
	const reduce = typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
	const d = (ms: number) => (reduce ? 0 : ms);

	const fmtDay = (s?: string | null) => (s ? new Date(s).toLocaleDateString(i18n.locale, { dateStyle: 'long' }) : '');
	const nama = $derived(profile?.full_name || profile?.email.split('@')[0] || '');
	const inisial = $derived(nama.slice(0, 2).toUpperCase());

	// ---------- skor keamanan ----------
	const checks = $derived(
		profile
			? [
					{ k: 'pr.check.name', ok: !!profile.full_name },
					{ k: 'pr.check.phone', ok: !!profile.phone },
					{ k: 'pr.check.email', ok: profile.email_verified },
					{ k: 'pr.check.mfa', ok: profile.mfa_enabled }
				]
			: []
	);
	const pct = $derived(Math.round((checks.filter((c) => c.ok).length / Math.max(1, checks.length)) * 100));
	const R = 42;
	const C = 2 * Math.PI * R;
	const tone = $derived(pct >= 100 ? 'text-emerald-500' : pct >= 50 ? 'text-emas' : 'text-red-400');
	const scoreLabel = $derived(pct >= 100 ? t('pr.score.ok') : pct >= 50 ? t('pr.score.mid') : t('pr.score.low'));

	// ---------- profil ----------
	// svelte-ignore state_referenced_locally
	let fullName = $state(profile?.full_name ?? '');
	// svelte-ignore state_referenced_locally
	let phone = $state(profile?.phone ?? '');
	let savingProfile = $state(false);
	let savedPulse = $state(false);
	const dirty = $derived(!!profile && (fullName.trim() !== profile.full_name || phone.trim() !== profile.phone));

	// ---------- kata sandi ----------
	let curPw = $state('');
	let newPw = $state('');
	let confPw = $state('');
	let showPw = $state(false);
	let savingPw = $state(false);
	const MIN = 12;
	const strength = $derived.by(() => {
		if (newPw.length < MIN) return 0;
		let s = 1;
		if (/[a-z]/.test(newPw) && /[A-Z]/.test(newPw)) s++;
		if (/\d/.test(newPw)) s++;
		if (/[^A-Za-z0-9]/.test(newPw) || newPw.length >= 20) s++;
		return Math.min(4, s);
	});
	const STR_COLOR = ['bg-red-500', 'bg-orange-500', 'bg-amber-400', 'bg-lime-500', 'bg-emerald-500'];
	const mismatch = $derived(confPw.length > 0 && confPw !== newPw);

	// ---------- 2FA ----------
	type Enroll = { secret: string; qr: string; codes: string[] };
	let enroll = $state<Enroll | null>(null);
	let step = $state(1);
	let starting = $state(false);
	let confirming = $state(false);
	let digits = $state<string[]>(['', '', '', '', '', '']);
	let boxes: HTMLInputElement[] = $state([]);
	let copied = $state('');
	let regenCodes = $state<string[] | null>(null);
	let justEnabled = $state(false);
	let badCode = $state(false);
	let proof = $state<null | 'disable' | 'regen'>(null);
	let proofErr = $state('');
	let proofBusy = $state(false);

	const otp = $derived(digits.join(''));
	function onDigit(i: number, e: Event) {
		const el = e.target as HTMLInputElement;
		const v = el.value.replace(/\D/g, '').slice(-1);
		digits[i] = v;
		el.value = v;
		if (v && i < 5) boxes[i + 1]?.focus();
	}
	function onKey(i: number, e: KeyboardEvent) {
		if (e.key === 'Backspace' && !digits[i] && i > 0) boxes[i - 1]?.focus();
		if (e.key === 'ArrowLeft' && i > 0) boxes[i - 1]?.focus();
		if (e.key === 'ArrowRight' && i < 5) boxes[i + 1]?.focus();
	}
	function onPaste(e: ClipboardEvent) {
		const txt = (e.clipboardData?.getData('text') ?? '').replace(/\D/g, '').slice(0, 6);
		if (!txt) return;
		e.preventDefault();
		digits = Array.from({ length: 6 }, (_, i) => txt[i] ?? '');
		boxes[Math.min(txt.length, 5)]?.focus();
	}
	async function copy(text: string, key: string) {
		try {
			await navigator.clipboard.writeText(text);
			copied = key;
			setTimeout(() => (copied = ''), 1600);
		} catch {
			/* clipboard bisa diblokir; abaikan */
		}
	}
	function downloadCodes(codes: string[]) {
		const body = `EraPay — kode pemulihan 2FA (${profile?.email})\nSekali pakai. Simpan di tempat aman.\n\n${codes.join('\n')}\n`;
		const a = document.createElement('a');
		a.href = URL.createObjectURL(new Blob([body], { type: 'text/plain' }));
		a.download = 'erapay-kode-pemulihan.txt';
		a.click();
		URL.revokeObjectURL(a.href);
	}
	function resetEnroll() {
		enroll = null;
		step = 1;
		digits = ['', '', '', '', '', ''];
		badCode = false;
	}

	// Pembungkus use:enhance: jalankan handler dengan data hasil, lalu segarkan data halaman tanpa mereset isian.
	const run = (busy: (v: boolean) => void, handler: (res: any, kind: string) => void | Promise<void>) => () => {
		busy(true);
		return async ({ result, update }: { result: any; update: (o?: { reset?: boolean }) => Promise<void> }) => {
			if (result.type === 'success' || result.type === 'failure') await handler(result.data ?? {}, result.type);
			else if (result.type === 'error') notify('danger', t('pr.err'));
			busy(false);
			await update({ reset: false });
		};
	};

	const onProfile = (res: any, kind: string) => {
		if (kind === 'failure') return notify('danger', res.message ?? t('pr.err'));
		notify('success', t('pr.saved'));
		savedPulse = true;
		setTimeout(() => (savedPulse = false), 1400);
	};
	const onPassword = (res: any, kind: string) => {
		if (kind === 'failure') {
			if (res.code === 'MISMATCH') return notify('danger', t('pr.pw.mismatch'));
			return notify('danger', res.message ?? t('pr.err'));
		}
		notify('success', t('pr.pw.ok'));
		curPw = newPw = confPw = '';
	};
	const onStart = (res: any, kind: string) => {
		if (kind === 'failure') return notify('danger', res.message ?? t('pr.err'));
		enroll = res.enroll;
		step = 1;
	};
	const onConfirm = (res: any, kind: string) => {
		if (kind === 'failure') {
			badCode = true;
			digits = ['', '', '', '', '', ''];
			boxes[0]?.focus();
			return notify('danger', t('pr.mfa.bad'));
		}
		justEnabled = true;
		resetEnroll();
		setTimeout(() => (justEnabled = false), 4200);
		notify('success', t('pr.mfa.okOn'));
	};
	const onProof = (res: any, kind: string) => {
		if (kind === 'failure') {
			proofErr = res.code === 'RATE_LIMITED' ? t('pr.mfa.rate') : t('pr.mfa.proofBad');
			return;
		}
		proofErr = '';
		if (res.regen) {
			regenCodes = res.regen;
			notify('success', t('pr.mfa.okRegen'));
		} else {
			notify('success', t('pr.mfa.okOff'));
		}
		proof = null;
	};

	const inputCls =
		'w-full rounded-xl border border-ink/15 bg-surface px-3.5 py-3 text-sm outline-none transition focus:border-merah focus:ring-4 focus:ring-merah/15';
	const btnCls =
		'inline-flex items-center justify-center gap-2 rounded-xl px-5 py-3 text-sm font-bold transition focus-visible:ring-4 focus-visible:ring-emas/60 disabled:cursor-not-allowed disabled:opacity-50';
</script>

<svelte:head><title>{t('pr.title')} · EraPay</title></svelte:head>

{#if !profile}
	<p class="rounded-xl border border-red-300 bg-red-50 px-4 py-3 text-xs font-semibold text-red-800" role="alert">{t('pr.loadFail')}</p>
{:else}
	<div class="mx-auto max-w-5xl space-y-6">
		<!-- ============ HERO ============ -->
		<header class="hero relative overflow-hidden rounded-3xl bg-gradient-to-br from-merah-gelap via-merah-tua to-merah p-6 text-white shadow-lg sm:p-8">
			<div class="pola pointer-events-none absolute inset-0 opacity-60"></div>
			<div class="orb orb-a"></div>
			<div class="orb orb-b"></div>
			<div class="relative flex flex-wrap items-center justify-between gap-6">
				<div class="flex min-w-0 items-center gap-5">
					<div class="avatar relative grid size-20 shrink-0 place-items-center rounded-full sm:size-24">
						<span class="avatar-ring"></span>
						<span class="relative grid size-[calc(100%-8px)] place-items-center rounded-full bg-merah-gelap font-display text-2xl font-extrabold sm:text-3xl">{inisial}</span>
					</div>
					<div class="min-w-0">
						<h1 class="font-display truncate text-2xl font-extrabold sm:text-3xl">{nama}</h1>
						<p class="mt-1 truncate text-sm text-white/80">{profile.email}</p>
						<div class="mt-3 flex flex-wrap gap-2">
							<span class="rounded-full px-3 py-1 text-[11px] font-bold {profile.email_verified ? 'bg-emerald-400/20 text-emerald-100' : 'bg-amber-400/25 text-amber-100'}">
								{profile.email_verified ? '✓ ' + t('pr.verified') : '! ' + t('pr.unverified')}
							</span>
							<span class="rounded-full bg-white/15 px-3 py-1 text-[11px] font-bold">{t('pr.since', { date: fmtDay(profile.created_at) })}</span>
						</div>
					</div>
				</div>

				<!-- cincin skor -->
				<div class="flex items-center gap-4 rounded-2xl bg-black/20 p-4 backdrop-blur-sm">
					<div class="relative size-24">
						<svg viewBox="0 0 100 100" class="-rotate-90" aria-hidden="true">
							<circle cx="50" cy="50" r={R} fill="none" stroke="currentColor" stroke-opacity="0.18" stroke-width="9" />
							<circle cx="50" cy="50" r={R} fill="none" stroke="currentColor" stroke-width="9" stroke-linecap="round" class="{tone} ring-fill" stroke-dasharray={C} stroke-dashoffset={C - (C * pct) / 100} />
						</svg>
						<div class="absolute inset-0 grid place-items-center text-center">
							<div><div class="font-mono text-xl font-bold">{pct}%</div></div>
						</div>
					</div>
					<div>
						<div class="text-[11px] font-bold tracking-wider text-white/70 uppercase">{t('pr.score')}</div>
						<div class="text-base font-extrabold">{scoreLabel}</div>
						<ul class="mt-1.5 space-y-0.5">
							{#each checks as c (c.k)}
								<li class="flex items-center gap-1.5 text-[11px] {c.ok ? 'text-emerald-200' : 'text-white/60'}">
									<span class="grid size-3.5 place-items-center rounded-full text-[9px] font-bold {c.ok ? 'bg-emerald-400 text-merah-gelap' : 'bg-white/20'}">{c.ok ? '✓' : ''}</span>
									{t(c.k as any)}
								</li>
							{/each}
						</ul>
					</div>
				</div>
			</div>
		</header>

		<!-- ============ TAB ============ -->
		<div class="card p-1.5" role="tablist">
			<div class="relative grid grid-cols-3 gap-1">
				<span class="tab-pill" style:transform={`translateX(calc(${TABS.indexOf(tab)} * (100% + 0.25rem)))`} aria-hidden="true"></span>
				{#each TABS as k (k)}
					<button
						type="button"
						role="tab"
						aria-selected={tab === k}
						class="relative z-10 flex items-center justify-center gap-2 rounded-xl px-3 py-3 text-xs font-bold transition aria-selected:text-white sm:text-sm {tab === k ? '' : 'text-ink/70 hover:text-ink'}"
						onclick={() => go(k)}
					>
						{#if k === 'profile'}
							<svg class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="8" r="4" /><path d="M4 21a8 8 0 0 1 16 0" /></svg>
						{:else if k === 'password'}
							<svg class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" viewBox="0 0 24 24" aria-hidden="true"><rect x="4" y="11" width="16" height="9" rx="2" /><path d="M8 11V8a4 4 0 0 1 8 0v3" /></svg>
						{:else}
							<svg class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3 5 6v6c0 4.2 2.8 7.4 7 9 4.2-1.6 7-4.8 7-9V6l-7-3Z" /><path d="m9 12 2 2 4-4" /></svg>
						{/if}
						<span class="truncate">{t(`pr.tab.${k}` as any)}</span>
						{#if k === 'mfa' && profile.mfa_enabled}<span class="size-2 rounded-full bg-emerald-400"></span>{/if}
					</button>
				{/each}
			</div>
		</div>

		<!-- ============ ISI ============ -->
		<div class="relative">
			{#key tab}
				<section in:fly={{ x: d(28) * dir, duration: d(320), delay: d(60) }} out:fade={{ duration: d(90) }} class="card p-6 sm:p-8">
					<!-- ---- PROFIL ---- -->
					{#if tab === 'profile'}
						<form method="POST" action="?/saveProfile" use:enhance={run((v) => (savingProfile = v), onProfile)} class="grid gap-6 sm:grid-cols-2">
							<div class="sm:col-span-2">
								<label class="mb-1.5 block text-xs font-bold" for="pr-name">{t('pr.f.name')}</label>
								<input id="pr-name" name="full_name" class={inputCls} placeholder={t('pr.f.namePh')} maxlength="100" autocomplete="name" bind:value={fullName} />
							</div>
							<div>
								<label class="mb-1.5 block text-xs font-bold" for="pr-email">{t('pr.f.email')}</label>
								<input id="pr-email" class="{inputCls} cursor-not-allowed bg-ink/5 font-mono text-xs" value={profile.email} readonly aria-describedby="pr-email-h" />
								<p id="pr-email-h" class="muted mt-1.5 text-[11px]">{t('pr.f.emailHint')}</p>
							</div>
							<div>
								<label class="mb-1.5 block text-xs font-bold" for="pr-phone">{t('pr.f.phone')}</label>
								<input id="pr-phone" name="phone" type="tel" class={inputCls} placeholder={t('pr.f.phonePh')} maxlength="32" autocomplete="tel" bind:value={phone} />
							</div>
							<div class="flex items-center gap-3 sm:col-span-2">
								<button type="submit" disabled={savingProfile || !dirty} class="{btnCls} btn-shine bg-merah text-white hover:bg-merah-tua">
									{#if savingProfile}<span class="spinner"></span>{t('pr.f.saving')}{:else}{t('pr.f.save')}{/if}
								</button>
								{#if savedPulse}
									<span in:scale={{ duration: d(260), start: 0.5 }} out:fade class="flex items-center gap-1.5 text-sm font-bold text-emerald-600">
										<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path class="draw" d="m5 12 5 5 9-10" /></svg>
										{t('pr.saved')}
									</span>
								{/if}
							</div>
						</form>
					{/if}

					<!-- ---- KATA SANDI ---- -->
					{#if tab === 'password'}
						<div class="grid gap-8 lg:grid-cols-[1fr_280px]">
							<form method="POST" action="?/changePassword" use:enhance={run((v) => (savingPw = v), onPassword)} class="space-y-5">
								<div>
									<h2 class="font-display text-lg font-extrabold">{t('pr.pw.title')}</h2>
									<p class="muted mt-1 text-xs">{t('pr.pw.hint')}</p>
								</div>
								{#each [['pr-cur', 'current_password', 'pr.pw.cur', 'current-password'], ['pr-new', 'new_password', 'pr.pw.new', 'new-password'], ['pr-conf', 'confirm_password', 'pr.pw.confirm', 'new-password']] as [id, name, label, ac] (id)}
									<div>
										<label class="mb-1.5 block text-xs font-bold" for={id}>{t(label as any)}</label>
										<div class="relative">
											<input
												{id}
												{name}
												type={showPw ? 'text' : 'password'}
												required
												autocomplete={ac as any}
												class="{inputCls} pr-20 {id === 'pr-conf' && mismatch ? 'border-red-500 focus:border-red-500 focus:ring-red-500/15' : ''}"
												value={id === 'pr-cur' ? curPw : id === 'pr-new' ? newPw : confPw}
												oninput={(e) => {
													const v = (e.target as HTMLInputElement).value;
													if (id === 'pr-cur') curPw = v;
													else if (id === 'pr-new') newPw = v;
													else confPw = v;
												}}
											/>
											{#if id === 'pr-cur'}
												<button type="button" class="absolute top-1/2 right-3 -translate-y-1/2 rounded-md px-2 py-1 text-[11px] font-bold text-merah hover:bg-merah/10" onclick={() => (showPw = !showPw)}>{showPw ? t('pr.pw.hide') : t('pr.pw.show')}</button>
											{/if}
										</div>
										{#if id === 'pr-new'}
											<div class="mt-2.5 flex items-center gap-3">
												<div class="flex flex-1 gap-1.5" aria-hidden="true">
													{#each [1, 2, 3, 4] as seg (seg)}
														<span class="h-1.5 flex-1 rounded-full transition-all duration-500 {strength >= seg ? STR_COLOR[strength] : 'bg-ink/10'}"></span>
													{/each}
												</div>
												<span class="w-20 text-right text-[11px] font-bold">{t(`pr.pw.s${strength}` as any)}</span>
											</div>
											<p class="muted mt-1 text-[11px]">{t('pr.pw.min', { n: MIN })}</p>
										{/if}
										{#if id === 'pr-conf' && mismatch}<p class="mt-1.5 text-[11px] font-semibold text-red-600" role="alert">{t('pr.pw.mismatch')}</p>{/if}
									</div>
								{/each}
								<button type="submit" disabled={savingPw || strength === 0 || mismatch || !confPw || !curPw} class="{btnCls} btn-shine bg-merah text-white hover:bg-merah-tua">
									{#if savingPw}<span class="spinner"></span>{/if}{t('pr.pw.save')}
								</button>
							</form>
							<aside class="relative hidden overflow-hidden rounded-2xl bg-gradient-to-br from-merah-gelap to-merah-tua p-6 text-white lg:block" aria-hidden="true">
								<div class="pola absolute inset-0 opacity-50"></div>
								<svg viewBox="0 0 64 64" class="float relative mx-auto size-24 text-emas" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><rect x="14" y="28" width="36" height="26" rx="5" /><path d="M22 28v-7a10 10 0 0 1 20 0v7" /><circle cx="32" cy="41" r="3.5" fill="currentColor" /></svg>
							</aside>
						</div>
					{/if}

					<!-- ---- 2FA ---- -->
					{#if tab === 'mfa'}
						<div class="flex flex-wrap items-start justify-between gap-4">
							<div class="max-w-xl">
								<h2 class="font-display text-lg font-extrabold">{t('pr.mfa.title')}</h2>
								<p class="muted mt-1 text-xs leading-relaxed">{t('pr.mfa.desc')}</p>
							</div>
							<span class="flex items-center gap-2 rounded-full px-3.5 py-1.5 text-[11px] font-bold {profile.mfa_enabled ? 'bg-emerald-500/15 text-emerald-700' : 'bg-ink/10 text-ink/70'}">
								<span class="relative flex size-2.5">
									{#if profile.mfa_enabled}<span class="absolute inline-flex size-full animate-ping rounded-full bg-emerald-400 opacity-60"></span>{/if}
									<span class="relative inline-flex size-2.5 rounded-full {profile.mfa_enabled ? 'bg-emerald-500' : 'bg-ink/40'}"></span>
								</span>
								{profile.mfa_enabled ? t('pr.mfa.on') : t('pr.mfa.off')}
							</span>
						</div>

						{#if justEnabled}
							<div in:scale={{ duration: d(380), start: 0.8 }} out:fade class="mt-6 flex items-center gap-4 rounded-2xl bg-emerald-500/10 p-5">
								<svg viewBox="0 0 52 52" class="size-14 shrink-0 text-emerald-500" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><circle class="draw-ring" cx="26" cy="26" r="23" /><path class="draw" d="m15 27 8 8 15-17" /></svg>
								<p class="text-sm font-bold text-emerald-800">{t('pr.mfa.okOn')}</p>
							</div>
						{/if}

						{#if profile.mfa_enabled && !enroll}
							<div in:fade={{ duration: d(200) }} class="mt-6 grid gap-4 sm:grid-cols-2">
								<div class="rounded-2xl border border-ink/10 bg-page p-5">
									<div class="flex items-center gap-3">
										<span class="shield grid size-12 place-items-center rounded-2xl bg-emerald-500/15 text-emerald-600">
											<svg viewBox="0 0 24 24" class="size-6" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3 5 6v6c0 4.2 2.8 7.4 7 9 4.2-1.6 7-4.8 7-9V6l-7-3Z" /><path d="m9 12 2 2 4-4" /></svg>
										</span>
										<div>
											<div class="text-sm font-bold">{t('pr.mfa.since', { date: fmtDay(profile.mfa_enrolled_at) })}</div>
											<div class="muted text-xs">{t('pr.mfa.left', { n: profile.recovery_codes_left })}</div>
										</div>
									</div>
								</div>
								<div class="flex flex-wrap items-center gap-3 sm:justify-end">
									<button type="button" class="{btnCls} border border-ink/15 bg-surface hover:bg-ink/5" onclick={() => ((proof = 'regen'), (proofErr = ''))}>{t('pr.mfa.regen')}</button>
									<button type="button" class="{btnCls} border border-red-600 bg-surface text-red-700 hover:bg-red-500/10" onclick={() => ((proof = 'disable'), (proofErr = ''))}>{t('pr.mfa.disable')}</button>
								</div>
							</div>

							{#if regenCodes}
								<div in:fly={{ y: d(16), duration: d(300) }} class="mt-6 rounded-2xl border border-emas/50 bg-emas/10 p-5">
									<div class="mb-3 flex flex-wrap items-center justify-between gap-2">
										<h3 class="text-sm font-bold">{t('pr.mfa.newCodes')}</h3>
										<div class="flex gap-2">
											<button type="button" class="rounded-lg border border-ink/15 bg-surface px-3 py-1.5 text-[11px] font-bold hover:bg-ink/5" onclick={() => copy(regenCodes!.join('\n'), 'regen')}>{copied === 'regen' ? t('pr.mfa.copied') : t('pr.mfa.copy')}</button>
											<button type="button" class="rounded-lg border border-ink/15 bg-surface px-3 py-1.5 text-[11px] font-bold hover:bg-ink/5" onclick={() => downloadCodes(regenCodes!)}>{t('pr.mfa.download')}</button>
											<button type="button" class="rounded-lg bg-ink/10 px-3 py-1.5 text-[11px] font-bold hover:bg-ink/15" onclick={() => (regenCodes = null)}>{t('pr.mfa.saved')}</button>
										</div>
									</div>
									<ul class="grid grid-cols-2 gap-2 sm:grid-cols-4">
										{#each regenCodes as c, i (c)}
											<li in:fly={{ y: d(10), duration: d(260), delay: d(i * 50) }} class="rounded-lg bg-surface px-3 py-2 text-center font-mono text-xs font-bold tracking-wider">{c}</li>
										{/each}
									</ul>
								</div>
							{/if}
						{:else if !enroll}
							<div in:fade={{ duration: d(200) }} class="mt-6 flex flex-wrap items-center gap-6 rounded-2xl border border-dashed border-ink/20 bg-page p-6">
								<div class="shield grid size-16 place-items-center rounded-2xl bg-merah/10 text-merah">
									<svg viewBox="0 0 24 24" class="size-8" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3 5 6v6c0 4.2 2.8 7.4 7 9 4.2-1.6 7-4.8 7-9V6l-7-3Z" /><rect x="9.5" y="10" width="5" height="4.5" rx="1" /><path d="M10.5 10V9a1.5 1.5 0 0 1 3 0v1" /></svg>
								</div>
								<form method="POST" action="?/mfaStart" use:enhance={run((v) => (starting = v), onStart)} class="flex-1">
									<button type="submit" disabled={starting} class="{btnCls} btn-shine bg-merah text-white hover:bg-merah-tua">
										{#if starting}<span class="spinner"></span>{t('pr.mfa.starting')}{:else}{t('pr.mfa.start')}{/if}
									</button>
								</form>
							</div>
						{:else}
							<!-- ---- wizard 3 langkah ---- -->
							<div class="mt-6">
								<div class="mb-5 flex items-center gap-3">
									{#each [1, 2, 3] as n (n)}
										<span class="grid size-8 place-items-center rounded-full text-xs font-bold transition-all duration-300 {step >= n ? 'bg-merah text-white' : 'bg-ink/10 text-ink/50'} {step === n ? 'scale-110 ring-4 ring-merah/20' : ''}">{step > n ? '✓' : n}</span>
										{#if n < 3}<span class="h-0.5 flex-1 overflow-hidden rounded-full bg-ink/10"><span class="block h-full bg-merah transition-all duration-500" style:width={step > n ? '100%' : '0%'}></span></span>{/if}
									{/each}
									<span class="muted ml-1 hidden text-[11px] font-bold sm:block">{t('pr.mfa.stepOf', { n: step })}</span>
								</div>

								{#key step}
									<div in:fly={{ x: d(24), duration: d(280) }} out:fade={{ duration: d(80) }}>
										{#if step === 1}
											<div class="grid items-center gap-6 sm:grid-cols-[auto_1fr]">
												<div class="qr relative mx-auto overflow-hidden rounded-2xl border border-ink/10 bg-white p-3 shadow-sm">
													<img src={enroll.qr} alt="QR 2FA" width="240" height="240" class="size-56 sm:size-60" />
													<span class="scan" aria-hidden="true"></span>
												</div>
												<div>
													<h3 class="text-base font-extrabold">{t('pr.mfa.step1')}</h3>
													<p class="muted mt-1 text-xs leading-relaxed">{t('pr.mfa.step1d')}</p>
													<p class="muted mt-4 text-[11px] font-bold">{t('pr.mfa.manual')}</p>
													<div class="mt-1.5 flex items-center gap-2">
														<code class="min-w-0 flex-1 truncate rounded-lg bg-ink/5 px-3 py-2 font-mono text-xs font-bold tracking-wider select-all">{enroll.secret}</code>
														<button type="button" class="shrink-0 rounded-lg border border-ink/15 px-3 py-2 text-[11px] font-bold hover:bg-ink/5" onclick={() => copy(enroll!.secret, 'secret')}>{copied === 'secret' ? t('pr.mfa.copied') : t('pr.mfa.copy')}</button>
													</div>
												</div>
											</div>
										{:else if step === 2}
											<h3 class="text-base font-extrabold">{t('pr.mfa.step2')}</h3>
											<p class="muted mt-1 text-xs leading-relaxed">{t('pr.mfa.step2d')}</p>
											<ul class="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4">
												{#each enroll.codes as c, i (c)}
													<li in:fly={{ y: d(10), duration: d(260), delay: d(i * 55) }} class="rounded-lg border border-ink/10 bg-page px-3 py-2.5 text-center font-mono text-xs font-bold tracking-wider">{c}</li>
												{/each}
											</ul>
											<div class="mt-4 flex flex-wrap gap-2">
												<button type="button" class="rounded-lg border border-ink/15 px-3.5 py-2 text-xs font-bold hover:bg-ink/5" onclick={() => copy(enroll!.codes.join('\n'), 'codes')}>{copied === 'codes' ? t('pr.mfa.copied') : t('pr.mfa.copy')}</button>
												<button type="button" class="rounded-lg border border-ink/15 px-3.5 py-2 text-xs font-bold hover:bg-ink/5" onclick={() => downloadCodes(enroll!.codes)}>{t('pr.mfa.download')}</button>
											</div>
										{:else}
											<form method="POST" action="?/mfaConfirm" use:enhance={run((v) => (confirming = v), onConfirm)}>
												<h3 class="text-base font-extrabold">{t('pr.mfa.step3')}</h3>
												<p class="muted mt-1 text-xs">{t('pr.mfa.step3d')}</p>
												<!-- svelte-ignore a11y_no_static_element_interactions -->
												<div class="mt-5 flex justify-center gap-2 sm:justify-start {badCode ? 'shake' : ''}" onpaste={onPaste} onanimationend={() => (badCode = false)}>
													{#each digits as _, i (i)}
														<input
															bind:this={boxes[i]}
															inputmode="numeric"
															autocomplete={i === 0 ? 'one-time-code' : 'off'}
															maxlength="1"
															aria-label={`${i + 1}`}
															value={digits[i]}
															oninput={(e) => onDigit(i, e)}
															onkeydown={(e) => onKey(i, e)}
															class="size-12 rounded-xl border bg-surface text-center font-mono text-xl font-bold outline-none transition focus:scale-105 focus:border-merah focus:ring-4 focus:ring-merah/15 {digits[i] ? 'border-merah' : 'border-ink/15'} {badCode ? 'border-red-500' : ''}"
														/>
													{/each}
												</div>
												<input type="hidden" name="code" value={otp} />
												<button type="submit" disabled={confirming || otp.length !== 6} class="{btnCls} btn-shine mt-6 bg-merah text-white hover:bg-merah-tua">
													{#if confirming}<span class="spinner"></span>{/if}{t('pr.mfa.confirm')}
												</button>
											</form>
										{/if}
									</div>
								{/key}

								<div class="mt-6 flex items-center justify-between border-t border-ink/10 pt-4">
									<button type="button" class="rounded-lg px-3 py-2 text-xs font-bold text-ink/70 hover:bg-ink/5" onclick={() => (step === 1 ? resetEnroll() : (step -= 1))}>{step === 1 ? t('pr.mfa.cancel') : t('pr.mfa.back')}</button>
									{#if step < 3}
										<button type="button" class="{btnCls} bg-merah py-2.5 text-white hover:bg-merah-tua" onclick={() => (step += 1)}>{step === 2 ? t('pr.mfa.saved') : t('pr.mfa.next')} →</button>
									{/if}
								</div>
							</div>
						{/if}
					{/if}
				</section>
			{/key}
		</div>
	</div>

	<!-- ============ KONFIRMASI IDENTITAS (nonaktifkan / buat ulang) ============ -->
	{#if proof}
		<Modal title={proof === 'disable' ? t('pr.mfa.proofTitle.disable') : t('pr.mfa.proofTitle.regen')} onclose={() => (proof = null)}>
			<form method="POST" action={proof === 'disable' ? '?/mfaDisable' : '?/mfaRegen'} use:enhance={run((v) => (proofBusy = v), onProof)} class="space-y-4">
				<p class="muted text-xs leading-relaxed">{t('pr.mfa.proofDesc')}</p>
				<div>
					<label class="mb-1.5 block text-xs font-bold" for="pf-pw">{t('pr.mfa.password')}</label>
					<input id="pf-pw" name="password" type="password" required autocomplete="current-password" class={inputCls} />
				</div>
				<div>
					<label class="mb-1.5 block text-xs font-bold" for="pf-code">{t('pr.mfa.code')}</label>
					<input id="pf-code" name="code" required inputmode="text" autocomplete="one-time-code" class="{inputCls} font-mono tracking-widest" />
				</div>
				{#if proofErr}<p class="shake text-xs font-semibold text-red-600" role="alert">{proofErr}</p>{/if}
				<div class="flex justify-end gap-2 pt-1">
					<button type="button" class="{btnCls} border border-ink/15 bg-surface py-2.5 hover:bg-ink/5" onclick={() => (proof = null)}>{t('pr.mfa.cancel')}</button>
					<button type="submit" disabled={proofBusy} class="{btnCls} py-2.5 text-white {proof === 'disable' ? 'bg-red-600 hover:bg-red-700' : 'bg-merah hover:bg-merah-tua'}">
						{#if proofBusy}<span class="spinner"></span>{/if}{proof === 'disable' ? t('pr.mfa.disable') : t('pr.mfa.regen')}
					</button>
				</div>
			</form>
		</Modal>
	{/if}
{/if}

<style>
	/* Hero: cincin avatar berputar, orb melayang, cincin skor terisi saat muncul. */
	.avatar-ring {
		position: absolute;
		inset: 0;
		border-radius: 9999px;
		background: conic-gradient(from 0deg, #e8b84a, #fff3c4, #e8b84a, #c8102e, #e8b84a);
		animation: spin 6s linear infinite;
	}
	.orb {
		position: absolute;
		border-radius: 9999px;
		filter: blur(40px);
		opacity: 0.45;
		pointer-events: none;
	}
	.orb-a { width: 14rem; height: 14rem; right: -3rem; top: -5rem; background: #e8b84a; animation: drift 9s ease-in-out infinite; }
	.orb-b { width: 11rem; height: 11rem; left: 20%; bottom: -6rem; background: #ff5a72; animation: drift 11s ease-in-out infinite reverse; }
	.hero { animation: rise 0.6s cubic-bezier(0.2, 0.7, 0.2, 1) both; }
	.ring-fill { transition: stroke-dashoffset 1.2s cubic-bezier(0.2, 0.8, 0.2, 1); animation: ring-in 1.4s cubic-bezier(0.2, 0.8, 0.2, 1) both; }

	/* Pil tab bergeser mulus. */
	.tab-pill {
		position: absolute;
		inset: 0 auto 0 0;
		width: calc((100% - 0.5rem) / 3);
		border-radius: 0.75rem;
		background: linear-gradient(135deg, #c8102e, #8a0a1f);
		box-shadow: 0 6px 16px -6px rgba(200, 16, 46, 0.6);
		transition: transform 0.4s cubic-bezier(0.3, 1.2, 0.4, 1);
	}

	/* Tombol berkilau saat hover. */
	.btn-shine { position: relative; overflow: hidden; }
	.btn-shine::after {
		content: '';
		position: absolute;
		inset: 0;
		background: linear-gradient(110deg, transparent 30%, rgba(255, 255, 255, 0.35) 50%, transparent 70%);
		transform: translateX(-120%);
	}
	.btn-shine:not(:disabled):hover::after { transform: translateX(120%); transition: transform 0.7s ease; }

	.spinner { width: 1rem; height: 1rem; border-radius: 9999px; border: 2px solid rgba(255, 255, 255, 0.4); border-top-color: #fff; animation: spin 0.7s linear infinite; }

	/* QR: garis pindai naik-turun. */
	.scan {
		position: absolute;
		left: 0.75rem;
		right: 0.75rem;
		height: 2px;
		background: linear-gradient(90deg, transparent, #c8102e, transparent);
		box-shadow: 0 0 12px 2px rgba(200, 16, 46, 0.5);
		animation: scan 2.4s ease-in-out infinite;
	}

	.shield { animation: float 3.2s ease-in-out infinite; }
	.float { animation: float 3.6s ease-in-out infinite; }
	.shake { animation: shake 0.45s ease-in-out; }
	.draw { stroke-dasharray: 40; stroke-dashoffset: 40; animation: draw 0.5s ease 0.2s forwards; }
	.draw-ring { stroke-dasharray: 150; stroke-dashoffset: 150; animation: draw 0.7s ease forwards; }

	@keyframes spin { to { transform: rotate(360deg); } }
	@keyframes drift { 0%, 100% { transform: translate(0, 0); } 50% { transform: translate(-20px, 18px); } }
	@keyframes rise { from { opacity: 0; transform: translateY(18px); } to { opacity: 1; transform: none; } }
	@keyframes ring-in { from { stroke-dashoffset: 264; } }
	@keyframes scan { 0%, 100% { top: 0.75rem; opacity: 0; } 10%, 90% { opacity: 1; } 50% { top: calc(100% - 0.85rem); } }
	@keyframes float { 0%, 100% { transform: translateY(0); } 50% { transform: translateY(-6px); } }
	@keyframes shake { 0%, 100% { transform: translateX(0); } 20%, 60% { transform: translateX(-7px); } 40%, 80% { transform: translateX(7px); } }
	@keyframes draw { to { stroke-dashoffset: 0; } }

	@media (prefers-reduced-motion: reduce) {
		.avatar-ring, .orb, .hero, .ring-fill, .scan, .shield, .float, .shake, .draw, .draw-ring, .spinner { animation: none !important; }
		.draw, .draw-ring { stroke-dashoffset: 0; }
		.tab-pill { transition: none; }
	}
</style>
