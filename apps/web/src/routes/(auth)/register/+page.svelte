<script lang="ts">
	import { enhance } from '$app/forms';
	import { getContext, untrack } from 'svelte';
	import { getI18n } from '../../../lib/i18n/i18n.svelte';
	import Field from '../../../lib/components/auth/Field.svelte';
	import Logo from '../../../lib/components/auth/Logo.svelte';

	let { form } = $props();
	const auth = getContext<{ goyang(): void; uang(): void; pindah(href: string): void }>('auth');
	const { t } = getI18n();

	let jenis = $state('pribadi');
	let nama = $state(untrack(() => form?.name ?? ''));
	let usaha = $state(untrack(() => form?.business_name ?? ''));
	let email = $state(untrack(() => form?.email ?? ''));
	let hp = $state('');
	let nik = $state('');
	let pob = $state('');
	let dob = $state('');
	let gender = $state('');
	let alamat = $state('');
	let npwp = $state('');
	let bizNpwp = $state('');
	let nib = $state('');
	let sandi = $state('');
	let sandi2 = $state('');
	let setuju = $state(false);
	let showPw = $state(false);
	let busy = $state(false);
	let err = $state<Record<string, string>>({});

	const digits = (v: string) => v.replace(/\D/g, '');
	// Usia minimal pemilik KTP: 17 tahun. Tanggal lahir lebih baru dari ini ditolak.
	const dobMax = new Date(Date.now() - 17 * 365.25 * 86_400_000).toISOString().slice(0, 10);

	const TEKS = $derived([t('reg.s0'), t('reg.s1'), t('reg.s2'), t('reg.s3'), t('reg.s4')]);
	const WARNA = ['#ef4444', '#ef4444', '#f59e0b', '#84cc16', '#16a34a'];
	const skor = $derived.by(() => {
		let s = 0;
		if (sandi.length >= 12) s++;
		if (/[a-z]/.test(sandi) && /[A-Z]/.test(sandi)) s++;
		if (/\d/.test(sandi) && /[^A-Za-z0-9]/.test(sandi)) s++;
		if (sandi.length >= 16) s++;
		return s;
	});

	$effect(() => {
		if (form?.error) auth.goyang();
	});
	$effect(() => {
		if (form?.success) auth.uang();
	});

	function validate() {
		const nomor = hp.replace(/\D/g, '').replace(/^(62|0)/, '');
		const errs: Record<string, string> = {
			nama: nama.trim().length >= 3 ? '' : t('reg.err.name'),
			email: /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim()) ? '' : t('auth.err.email'),
			hp: /^8\d{8,11}$/.test(nomor) ? '' : t('reg.err.phone'),
			usaha: jenis !== 'usaha' || usaha.trim().length >= 3 ? '' : t('reg.err.business'),
			nik: /^\d{16}$/.test(digits(nik)) ? '' : t('reg.err.nik'),
			pob: pob.trim().length >= 2 ? '' : t('reg.err.pob'),
			dob: dob && /^\d{4}-\d{2}-\d{2}$/.test(dob) && dob <= dobMax ? '' : t('reg.err.dob'),
			gender: gender ? '' : t('reg.err.gender'),
			alamat: alamat.trim().length >= 5 ? '' : t('reg.err.address'),
			npwp: !npwp || /^\d{15,16}$/.test(digits(npwp)) ? '' : t('reg.err.npwp'),
			sandi: sandi.length >= 12 && sandi.length <= 128 ? '' : t('reg.err.pw'),
			sandi2: sandi && sandi === sandi2 ? '' : t('reg.err.pw2'),
			setuju: setuju ? '' : t('reg.err.agree')
		};
		if (jenis === 'usaha') {
			errs.bizNpwp = /^\d{15,16}$/.test(digits(bizNpwp)) ? '' : t('reg.err.bizNpwp');
			errs.nib = /^\d{13}$/.test(digits(nib)) ? '' : t('reg.err.nib');
		}
		err = errs;
		return Object.values(err).every((v) => !v);
	}
</script>

<svelte:head><title>{t('reg.title')}</title></svelte:head>

{#if form?.success}
	<div class="py-6 text-center">
		<div class="animate-naik mx-auto grid size-20 place-items-center rounded-full bg-emerald-500 text-white shadow-lg shadow-emerald-500/40">
			<svg viewBox="0 0 24 24" class="size-10" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12.5l4.5 4.5L19 7.5" /></svg>
		</div>
		<h2 class="animate-naik font-display mt-5 text-2xl font-extrabold text-merah-gelap [animation-delay:.1s]">{t('reg.okTitle')}</h2>
		<p class="animate-naik mt-2 text-sm/6 text-neutral-600 [animation-delay:.2s]">{t('reg.okThanks')} <b>{form.name}</b>.<br />{t('reg.okSent')} <b>{form.email}</b>. {t('reg.okRest')}</p>
		<a
			href="/login"
			onclick={(e) => { e.preventDefault(); auth.pindah('/login'); }}
			class="animate-naik mt-6 block w-full rounded-xl bg-gradient-to-b from-merah to-merah-tua px-5 py-3.5 font-bold text-white shadow-lg shadow-merah/40 transition hover:-translate-y-0.5 [animation-delay:.3s]"
		>{t('reg.okGo')}</a>
	</div>
{:else}
	<div class="flex justify-center lg:hidden">
		<Logo class="h-16" />
	</div>

	<p class="mt-5 text-sm font-semibold tracking-widest text-merah uppercase lg:mt-0">{t('reg.eyebrow')}</p>
	<h2 class="font-display mt-1 text-3xl font-extrabold text-merah-gelap">{t('reg.heading')}</h2>
	<p class="mt-2 text-sm text-neutral-500">{t('reg.sub')}</p>

	<form
		method="POST"
		novalidate
		class="mt-6 space-y-4"
		use:enhance={({ cancel }) => {
			if (!validate()) {
				auth.goyang();
				cancel();
				return;
			}
			busy = true;
			return async ({ update }) => {
				await update({ reset: false });
				busy = false;
			};
		}}
	>
		{#if form?.error}
			<div class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800" role="alert">{form.error}</div>
		{/if}

		<div class="grid grid-cols-2 gap-1 rounded-xl bg-neutral-200/70 p-1" role="radiogroup" aria-label={t('reg.type')}>
			{#each [['pribadi', t('reg.personal')], ['usaha', t('reg.business')]] as [v, jenisLabel]}
				<label class="cursor-pointer">
					<input type="radio" name="jenis" value={v} bind:group={jenis} class="peer sr-only" />
					<span class="flex items-center justify-center rounded-lg py-2 text-sm font-semibold text-neutral-600 transition peer-checked:bg-white peer-checked:text-merah peer-checked:shadow peer-focus-visible:outline-3 peer-focus-visible:outline-emas">{jenisLabel}</span>
				</label>
			{/each}
		</div>

		{#if jenis === 'usaha'}
			<Field compact id="r-usaha" name="business_name" label={t('reg.bizName')} icon="store" autocomplete="organization" placeholder={t('reg.bizNamePh')} bind:value={usaha} error={err.usaha} />
		{/if}
		<Field compact id="r-nama" name="name" label={t('reg.name')} icon="user" autocomplete="name" placeholder={t('reg.namePh')} bind:value={nama} error={err.nama} />
		<Field compact id="r-email" name="email" label="Email" icon="mail" type="email" autocomplete="email" placeholder="nama@usaha.co.id" bind:value={email} error={err.email} />
		<Field compact id="r-hp" name="phone" label={t('reg.phone')} icon="phone" prefix="+62" type="tel" autocomplete="tel-national" placeholder="812 3456 7890" bind:value={hp} error={err.hp} />

		<div>
			<p class="text-[11px] font-bold uppercase tracking-wider text-merah/80">{t('reg.dataDiri')}</p>
			<p class="text-[11px] text-neutral-500">{t('reg.dataDiriNote')}</p>
		</div>

		{#if jenis === 'usaha'}
			<Field compact id="r-biznpwp" name="biz_npwp" label={t('reg.bizNpwp')} icon="card" inputmode="numeric" placeholder={t('reg.npwpPh')} bind:value={bizNpwp} error={err.bizNpwp} />
			<Field compact id="r-nib" name="nib" label={t('reg.nib')} icon="card" inputmode="numeric" placeholder={t('reg.nibPh')} bind:value={nib} error={err.nib} />
		{/if}
		<Field compact id="r-nik" name="nik" label={t('reg.nik')} icon="card" inputmode="numeric" placeholder={t('reg.nikPh')} bind:value={nik} error={err.nik} />

		<div class="grid gap-3 sm:grid-cols-2">
			<Field compact id="r-pob" name="pob" label={t('reg.pob')} icon="pin" placeholder={t('reg.pobPh')} bind:value={pob} error={err.pob} />
			<div>
				<label for="r-dob" class="mb-1 block text-sm font-semibold text-neutral-700">{t('reg.dob')}</label>
				<input
					id="r-dob" name="dob" type="date" max={dobMax} autocomplete="bday" required
					bind:value={dob}
					aria-invalid={err.dob ? 'true' : undefined}
					class="w-full rounded-xl border bg-white py-2.5 px-4 text-sm shadow-sm transition outline-none placeholder:text-neutral-400 focus:border-merah focus:ring-4 focus:ring-merah/15 {err.dob ? 'border-merah' : 'border-neutral-300'}"
				/>
				{#if err.dob}<p id="r-dob-err" class="mt-1.5 text-xs font-medium text-merah">{err.dob}</p>{/if}
			</div>
		</div>

		<div>
			<span class="mb-1 block text-sm font-semibold text-neutral-700" id="r-gender-label">{t('reg.gender')}</span>
			<div class="grid grid-cols-2 gap-1 rounded-xl bg-neutral-200/70 p-1" role="radiogroup" aria-labelledby="r-gender-label">
				{#each [['male', t('reg.male')], ['female', t('reg.female')]] as [v, kelamin]}
					<label class="cursor-pointer">
						<input type="radio" name="gender" value={v} bind:group={gender} class="peer sr-only" />
						<span class="flex items-center justify-center rounded-lg py-2 text-sm font-semibold text-neutral-600 transition peer-checked:bg-white peer-checked:text-merah peer-checked:shadow peer-focus-visible:outline-3 peer-focus-visible:outline-emas">{kelamin}</span>
					</label>
				{/each}
			</div>
			{#if err.gender}<p class="mt-1.5 text-xs font-medium text-merah">{err.gender}</p>{/if}
		</div>

		<Field compact id="r-alamat" name="address" label={t('reg.address')} icon="pin" placeholder={t('reg.addressPh')} bind:value={alamat} error={err.alamat} />

		<Field compact id="r-npwp" name="npwp" label={t('reg.npwp')} icon="card" inputmode="numeric" placeholder={t('reg.npwpPh')} bind:value={npwp} error={err.npwp} />

		<Field
			compact id="r-sandi" name="password" label={t('login.pw')} icon="lock"
			type={showPw ? 'text' : 'password'} autocomplete="new-password" placeholder={t('reg.pwPh')}
			bind:value={sandi} error={err.sandi}
		>
			{#snippet trailing()}
				<button
					type="button"
					onclick={() => (showPw = !showPw)}
					aria-label={showPw ? t('auth.pw.hide') : t('auth.pw.show')}
					aria-pressed={showPw}
					class="absolute inset-y-0 right-3 grid place-items-center rounded-lg px-1.5 text-neutral-400 transition hover:text-merah"
				>
					<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
						{#if showPw}
							<path d="M3 3l18 18M10.6 5.1A9.9 9.9 0 0 1 12 5c6.5 0 10 7 10 7a17 17 0 0 1-3.2 4M6.5 6.6C3.7 8.3 2 12 2 12s3.5 7 10 7c1.7 0 3.2-.4 4.5-1M9.9 9.9a3 3 0 0 0 4.2 4.2" />
						{:else}
							<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" /><circle cx="12" cy="12" r="3" />
						{/if}
					</svg>
				</button>
			{/snippet}
			{#snippet below()}
				<div class="mt-2 flex items-center gap-3" aria-live="polite">
					<div class="grid flex-1 grid-cols-4 gap-1.5">
						{#each [0, 1, 2, 3] as i}
							<span
								class="h-1.5 rounded-full bg-neutral-200 transition-colors duration-300"
								style:background={sandi && i < Math.max(1, skor) ? WARNA[skor] : ''}
							></span>
						{/each}
					</div>
					<span class="w-24 text-right text-xs font-semibold text-neutral-400" style:color={sandi ? WARNA[skor] : ''}>{sandi ? TEKS[skor] : '—'}</span>
				</div>
			{/snippet}
		</Field>

		<Field
			compact id="r-sandi2" name="password_confirm" label={t('reg.pw2')} icon="lock"
			type={showPw ? 'text' : 'password'} autocomplete="new-password" placeholder={t('reg.pw2Ph')}
			bind:value={sandi2} error={err.sandi2}
		/>

		<div>
			<label class="flex cursor-pointer items-start gap-2.5 text-xs/5 text-neutral-600 select-none">
				<input type="checkbox" name="agree" bind:checked={setuju} class="mt-0.5 size-4 shrink-0 rounded border-neutral-300 accent-merah" />
				<span>{t('reg.agree1')} <a href="/" class="font-semibold text-merah hover:underline">{t('reg.terms')}</a> {t('reg.and')} <a href="/" class="font-semibold text-merah hover:underline">{t('reg.privacy')}</a> {t('reg.agree2')}</span>
			</label>
			{#if err.setuju}<p class="mt-1 text-xs font-medium text-merah">{err.setuju}</p>{/if}
		</div>

		<button
			type="submit"
			disabled={busy}
			class="tombol-kilau relative flex w-full items-center justify-center gap-2 overflow-hidden rounded-xl bg-gradient-to-b from-merah to-merah-tua px-5 py-3.5 font-bold text-white shadow-lg shadow-merah/40 transition hover:-translate-y-0.5 hover:shadow-xl hover:shadow-merah/50 active:translate-y-0 disabled:opacity-90"
		>
			{#if busy}
				<svg class="size-5 animate-spin" viewBox="0 0 24 24" fill="none" aria-hidden="true"><circle cx="12" cy="12" r="9" stroke="currentColor" stroke-opacity=".3" stroke-width="3" /><path d="M21 12a9 9 0 0 0-9-9" stroke="currentColor" stroke-width="3" stroke-linecap="round" /></svg>
			{/if}
			<span class="relative">{busy ? t('reg.busy') : t('reg.submit')}</span>
		</button>
	</form>

	<p class="mt-5 text-center text-sm text-neutral-600">
		{t('reg.hasAccount')}
		<a
			href="/login"
			onclick={(e) => { e.preventDefault(); auth.pindah('/login'); }}
			class="font-bold text-merah hover:underline"
		>{t('reg.login')}</a>
	</p>
{/if}
