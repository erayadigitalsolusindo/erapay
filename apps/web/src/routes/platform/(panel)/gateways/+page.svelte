<script lang="ts">
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';
	import GatewayLogo from '../../../../lib/components/GatewayLogo.svelte';
	import type { CredEnv, CredGateway, CredStep } from './+page.server';

	let { data, form } = $props();
	const { tx } = getI18n();

	const STEP: Record<string, string> = { credentials: 'Kredensial gateway (token berhasil diminta)' };
	const fmt = (s?: string) => (s ? new Date(s).toLocaleString('id-ID', { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—');

	const fm = $derived(form as { key?: string; error?: string; saved?: boolean; result?: { ok: boolean; applied: boolean; steps: CredStep[] } } | null);
	const serverEnv = $derived(data.overview?.server_environment ?? '');
	let busy = $state<string | null>(null);

	const keyOf = (g: CredGateway, e: CredEnv) => `${g.gateway}/${e.environment}`;

	const rf = $derived(form as { key?: string; error?: string; routed?: string } | null);
	const routing = $derived(data.routing);
	const LABEL: Record<string, string> = { doku: 'DOKU', ids: 'IDS Payments' };
	// Pilihan di form; mengikuti pilihan tersimpan sampai admin mengubahnya.
	let pick = $state('');
	$effect(() => {
		pick = routing?.gateway ?? '';
	});
	const mineRoute = $derived(rf?.key === 'route');
</script>

<svelte:head><title>{tx('pf.nav.gateways', 'Gateway')}</title></svelte:head>

<div class="space-y-6">
	<section class="relative overflow-hidden rounded-3xl bg-linear-to-br from-merah-gelap via-merah-tua to-merah p-6 text-white shadow-lg sm:p-8">
		<div class="pointer-events-none absolute -right-16 -top-20 size-64 rounded-full bg-emas/25 blur-3xl"></div>
		<p class="relative text-[10px] font-bold uppercase tracking-[.25em] text-white/60">DEWA · {tx('pf.gw.kicker', 'Gateway pembayaran')}</p>
		<h1 class="relative mt-1 font-display text-3xl font-extrabold">{tx('pf.gc.title', 'Kredensial gateway')}</h1>
		<p class="relative mt-2 max-w-2xl text-[13px] leading-relaxed text-white/75">
			{tx('pf.gc.lead', 'Kelola Client ID, secret, dan private key gateway dari sini tanpa mengedit .env di server. Nilai disimpan terenkripsi dan tidak pernah ditampilkan lagi setelah disimpan.')}
		</p>
	</section>

	{#if routing}
		<section class="card overflow-hidden" aria-labelledby="route-title">
			<div class="space-y-1 p-4 sm:p-5">
				<h2 id="route-title" class="font-display text-lg font-extrabold uppercase">{tx('pf.rt.title', 'Jalur pembayaran')}</h2>
				<p class="text-xs leading-relaxed text-ink/70">
					{tx('pf.rt.lead', 'Pilih gateway yang menerima transaksi baru. Berlaku untuk pembayaran berikutnya; transaksi yang sudah dibuat tetap di gateway-nya. Kalau gateway terpilih tidak bisa melayani seorang merchant, transaksi itu memakai akun gateway milik merchant tersebut.')}
				</p>
				<p class="text-[11px] font-semibold text-ink/60">
					{#if routing.gateway}
						{tx('pf.rt.now', 'Dipakai sekarang')}: <b>{LABEL[routing.gateway] ?? routing.gateway}</b> · v{routing.version} · {fmt(routing.updated_at)}
					{:else}
						{tx('pf.rt.none', 'Belum dipilih: tiap merchant memakai akun gateway aktifnya sendiri (biasanya DOKU).')}
					{/if}
				</p>
			</div>
			<form
				method="POST"
				action="?/route"
				autocomplete="off"
				use:enhance={() => {
					busy = 'route';
					return async ({ update }) => {
						await update({ reset: false });
						busy = null;
					};
				}}
				class="space-y-3 border-t border-ink/10 p-4 sm:p-5"
			>
				<input type="hidden" name="environment" value={routing.server_environment} />
				<input type="hidden" name="version" value={routing.version} />
				<div class="grid gap-2 sm:grid-cols-2" role="radiogroup" aria-label={tx('pf.rt.title', 'Jalur pembayaran')}>
					{#each routing.options as o (o.code)}
						<label class="flex cursor-pointer items-start gap-3 rounded-2xl border p-3.5 transition {pick === o.code ? 'border-merah bg-merah/5' : 'border-ink/10 hover:bg-ink/[.03]'} {o.available ? '' : 'cursor-not-allowed opacity-60'}">
							<input type="radio" name="gateway" value={o.code} bind:group={pick} disabled={!o.available} class="mt-1" />
							<span class="min-w-0 flex-1">
								<span class="flex flex-wrap items-center gap-2">
									<b class="text-sm">{LABEL[o.code] ?? o.code}</b>
									{#if routing.gateway === o.code}<span class="pill bg-merah/10 text-merah">{tx('pf.rt.current', 'Terpilih')}</span>{/if}
									<span class="pill {o.available ? 'bg-emerald-500/15 text-emerald-700' : 'bg-amber-500/15 text-amber-700'}">{o.available ? tx('pf.rt.ready', 'Siap') : tx('pf.rt.notReady', 'Belum aktif di server')}</span>
								</span>
								<span class="muted mt-1 block text-[11px] leading-snug">
									{#if !o.available}
										{tx('pf.rt.needCreds', 'Simpan lalu Terapkan kredensialnya di bawah dulu.')}
									{:else if o.platform_account}
										{tx('pf.rt.platformAcct', 'Satu akun untuk seluruh platform: merchant yang sudah disetujui tidak perlu akun gateway sendiri.')}
									{:else}
										{tx('pf.rt.merchantAcct', 'Hanya merchant dengan akun gateway aktif yang dilayani; lainnya memakai akunnya sendiri.')}
									{/if}
								</span>
							</span>
						</label>
					{/each}
				</div>
				<div>
					<label class="lbl mb-1" for="route-note">{tx('pf.gw.fNote', 'Alasan (wajib, masuk audit)')}</label>
					<input id="route-note" name="note" required minlength="3" maxlength="500" class="inp" placeholder="mis. DOKU sedang gangguan" />
				</div>
				<div class="flex flex-wrap items-center gap-3">
					<button disabled={busy !== null || !pick || pick === routing.gateway} class="rounded-full bg-merah px-5 py-2 text-xs font-bold text-white shadow-sm transition hover:bg-merah-tua disabled:opacity-50">
						{busy === 'route' ? 'Menyimpan…' : tx('pf.rt.save', 'Simpan jalur')}
					</button>
					{#if mineRoute && rf?.error}<p class="text-xs font-semibold text-red-700" role="alert">{rf.error}</p>{/if}
					{#if mineRoute && rf?.routed}<p class="text-xs font-semibold text-emerald-700" role="status">{tx('pf.rt.saved', 'Tersimpan. Transaksi berikutnya lewat')} {LABEL[rf.routed] ?? rf.routed}.</p>{/if}
				</div>
			</form>
		</section>
	{/if}

	{#if data.unavailable}
		<p class="rounded-xl border border-amber-300 bg-amber-50 px-4 py-3 text-xs font-semibold leading-relaxed text-amber-900" role="status">
			{tx('pf.gc.off', 'Endpoint kredensial gateway tidak ditemukan di API. Biasanya API belum di-restart setelah update; kalau sudah, cek ERAPAY_ENCRYPTION_KEY di .env server API. Server tetap memakai kredensial dari .env seperti biasa.')}
		</p>
	{:else if data.error}
		<p class="rounded-xl border border-red-300 bg-red-50 px-4 py-3 text-xs font-semibold text-red-800" role="alert">{data.error}</p>
	{:else if data.overview}
		<details class="card group overflow-hidden" open>
			<summary class="cursor-pointer list-none p-4 text-sm font-extrabold">{tx('pf.gc.howTitle', 'Cara kerja')}</summary>
			<ol class="list-decimal space-y-1.5 border-t border-ink/10 p-5 pl-9 text-xs leading-relaxed">
				<li><b>Simpan</b>: nilai dienkripsi dan disimpan di database. Belum mengubah apa pun yang sedang berjalan.</li>
				<li><b>Tes</b>: server mencoba meminta token ke gateway memakai kredensial tersimpan. Tidak ada uang bergerak.</li>
				<li><b>Terapkan</b>: hanya bisa jika tes lulus. Server langsung memakai kredensial baru tanpa restart; yang lama tetap dipakai kalau tes gagal.</li>
				<li>Selama belum ada kredensial tersimpan, server memakai <code class="font-mono">DOKU_*</code> dari .env seperti sebelumnya. Server berjalan di <b class="capitalize">{serverEnv}</b>; hanya environment itu yang bisa diuji dan diterapkan.</li>
			</ol>
		</details>

		{#each data.overview.gateways as g (g.gateway)}
			<details class="card group/gw overflow-hidden" open>
				<summary class="flex cursor-pointer list-none items-center gap-3.5 p-4 transition hover:bg-ink/[.03] sm:p-5 [&::-webkit-details-marker]:hidden">
					<GatewayLogo code={g.gateway} label={g.label} class="h-16" />
					<div class="min-w-0 flex-1">
						<h2 class="font-display text-lg font-extrabold uppercase">{g.label}</h2>
						<p class="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] font-semibold text-ink/60">
							{#each g.environments as e (e.environment)}
								<span class="inline-flex items-center gap-1.5 capitalize"><span class="size-2 rounded-full {e.complete ? 'bg-emerald-500' : e.configured ? 'bg-amber-500' : 'bg-ink/25'}"></span>{e.environment}: {e.complete ? 'lengkap' : e.configured ? 'belum lengkap' : 'memakai .env'}</span>
							{/each}
						</p>
					</div>
					<svg viewBox="0 0 24 24" class="size-5 shrink-0 text-ink/50 transition-transform group-open/gw:rotate-180" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m6 9 6 6 6-6" /></svg>
				</summary>
				<div class="space-y-3 border-t border-ink/10 p-4 sm:p-5">
					{#each g.environments as e (e.environment)}
						{@const key = keyOf(g, e)}
						{@const mine = fm?.key === key}
						<details class="group/env overflow-hidden rounded-2xl border border-ink/10 bg-ink/[.025]" open={e.environment === serverEnv || mine}>
							<summary class="flex cursor-pointer list-none flex-wrap items-center justify-between gap-2 px-4 py-3.5 transition hover:bg-ink/[.04] [&::-webkit-details-marker]:hidden">
								<div class="flex items-center gap-2">
									<span class="size-2.5 rounded-full {e.complete ? 'bg-emerald-500' : e.configured ? 'bg-amber-500' : 'bg-ink/25'}"></span>
									<h3 class="text-sm font-extrabold capitalize">{e.environment}</h3>
									{#if e.environment === serverEnv}<span class="pill bg-merah/10 text-merah">{tx('pf.gw.thisServer', 'Dipakai server ini')}</span>{/if}
								</div>
								<span class="pill {e.complete ? 'bg-emerald-500/15 text-emerald-700' : e.configured ? 'bg-amber-500/15 text-amber-700' : 'bg-ink/10 text-ink/60'}">
									{e.complete ? 'Lengkap' : e.configured ? 'Belum lengkap' : 'Belum diatur (memakai .env)'}
								</span>
								<svg viewBox="0 0 24 24" class="size-4 shrink-0 text-ink/50 transition-transform group-open/env:rotate-180" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m6 9 6 6 6-6" /></svg>
							</summary>
							<p class="muted border-t border-ink/10 px-4 pt-3 text-[11px]">v{e.version} · {tx('pf.gc.updated', 'diubah')} {fmt(e.updated_at)}</p>

							<form
								method="POST"
								action="?/save"
								autocomplete="off"
								use:enhance={() => {
									busy = `save:${key}`;
									return async ({ update }) => {
										await update({ reset: true });
										busy = null;
									};
								}}
								class="space-y-3 p-4"
							>
								<input type="hidden" name="gateway" value={g.gateway} />
								<input type="hidden" name="environment" value={e.environment} />
								<input type="hidden" name="version" value={e.version} />
								{#each e.fields as f (f.key)}
									<div>
										<label class="lbl mb-1" for="{key}-{f.key}">
											{f.label}{#if f.required}<span class="text-merah"> *</span>{/if}
										</label>
										{#if f.options}
											<select id="{key}-{f.key}" name="v:{f.key}" class="inp">
												<option value="">{f.set ? `Tetap (${f.preview})` : '— tidak diubah —'}</option>
												{#each f.options as o (o)}<option value={o}>{o}</option>{/each}
											</select>
										{:else if f.multiline}
											<textarea id="{key}-{f.key}" name="v:{f.key}" rows="4" spellcheck="false" autocomplete="off" class="inp font-mono text-[11px]"
												placeholder={f.set ? 'Tersimpan. Kosongkan untuk mempertahankan, tempel PEM baru untuk mengganti.' : '-----BEGIN PRIVATE KEY-----'}></textarea>
										{:else}
											<input id="{key}-{f.key}" name="v:{f.key}" type={f.secret ? 'password' : 'text'} spellcheck="false" autocomplete="off" class="inp font-mono"
												placeholder={f.set ? (f.secret ? `Tersimpan ${f.preview} · kosongkan untuk mempertahankan` : `Saat ini: ${f.preview}`) : ''} />
										{/if}
										<p class="muted mt-1 flex flex-wrap items-center gap-x-3 text-[11px] leading-snug">
											<span>{f.set ? '✓ terisi' : 'belum diisi'}{f.hint ? ` · ${f.hint}` : ''}</span>
											{#if f.set && !f.required}
												<label class="inline-flex items-center gap-1 font-semibold"><input type="checkbox" name="clear:{f.key}" value="1" /> hapus</label>
											{/if}
										</p>
									</div>
								{/each}
								<div>
									<label class="lbl mb-1" for="{key}-note">{tx('pf.gw.fNote', 'Alasan (wajib, masuk audit)')}</label>
									<input id="{key}-note" name="note" required minlength="3" maxlength="500" class="inp" placeholder="mis. rotasi secret key" />
								</div>
								<div class="flex flex-wrap items-center gap-3">
									<button disabled={busy !== null} class="rounded-full bg-merah px-5 py-2 text-xs font-bold text-white shadow-sm transition hover:bg-merah-tua disabled:opacity-50">
										{busy === `save:${key}` ? 'Menyimpan…' : 'Simpan'}
									</button>
									{#if mine && fm?.error}<p class="text-xs font-semibold text-red-700" role="alert">{fm.error}</p>{/if}
									{#if mine && fm?.saved}<p class="text-xs font-semibold text-emerald-700">Tersimpan. Jalankan Tes lalu Terapkan.</p>{/if}
								</div>
							</form>

							{#if e.environment === serverEnv}
								<div class="flex flex-wrap gap-2 border-t border-ink/10 px-4 py-3">
									{#each [{ action: 'test', label: 'Tes koneksi', cls: 'border border-sky-500/40 bg-sky-500/10 text-sky-700 hover:bg-sky-500/20' }, { action: 'apply', label: 'Terapkan ke server', cls: 'bg-ink text-surface hover:opacity-90' }] as b (b.action)}
										<form
											method="POST"
											action="?/{b.action}"
											use:enhance={() => {
												busy = `${b.action}:${key}`;
												return async ({ update }) => {
													await update({ reset: false });
													busy = null;
												};
											}}
										>
											<input type="hidden" name="gateway" value={g.gateway} />
											<input type="hidden" name="environment" value={e.environment} />
											<button disabled={busy !== null || !e.complete} title={e.complete ? '' : 'Lengkapi semua field wajib dulu'} class="rounded-full px-4 py-1.5 text-xs font-bold transition disabled:opacity-50 {b.cls}">
												{busy === `${b.action}:${key}` ? 'Memproses…' : b.label}
											</button>
										</form>
									{/each}
								</div>
							{:else}
								<p class="muted border-t border-ink/10 px-4 py-2.5 text-[11px]">{tx('pf.gw.otherEnv', 'Tidak bisa diuji dari server ini karena server berjalan di environment lain.')}</p>
							{/if}

							{#if mine && fm?.result}
								<div class="border-t border-ink/10 bg-surface px-4 py-4" aria-live="polite">
									<ul class="space-y-2">
										{#each fm.result.steps as s (s.name)}
											<li class="flex gap-2.5 text-xs">
												<span class="grid size-5 shrink-0 place-items-center rounded-full text-[10px] font-extrabold text-white {s.ok ? 'bg-emerald-500' : 'bg-red-500'}">{s.ok ? '✓' : '✕'}</span>
												<span>
													<b>{STEP[s.name] ?? s.name}</b>
													{#if s.message}<span class="mt-0.5 block rounded-lg bg-red-500/10 px-2 py-1 text-[11px] font-semibold text-red-800">{s.message}{s.code ? ` (${s.code})` : ''}</span>{/if}
												</span>
											</li>
										{/each}
									</ul>
									<p class="mt-3 rounded-xl px-3 py-2 text-xs font-extrabold {fm.result.ok ? 'bg-emerald-500/15 text-emerald-800' : 'bg-red-500/15 text-red-800'}">
										{#if fm.result.applied}Berhasil diterapkan. Server sekarang memakai kredensial ini.
										{:else if fm.result.ok}Koneksi berhasil. Klik Terapkan untuk memakainya.
										{:else}Koneksi gagal. Kredensial yang sedang berjalan tidak diubah.{/if}
									</p>
								</div>
							{/if}
						</details>
					{/each}
				</div>
			</details>
		{/each}
	{/if}
</div>
