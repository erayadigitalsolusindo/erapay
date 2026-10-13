"use client";
import { useEffect, useRef, useState } from "react";

/* ───────────── Tema: terang / gelap / auto ───────────── */
function useTheme() {
  const [mode, setMode] = useState(() => {
    try { return localStorage.getItem("batik-theme") || "auto"; } catch { return "auto"; }
  });
  useEffect(() => {
    const mq = matchMedia("(prefers-color-scheme: dark)");
    const apply = () =>
      document.documentElement.classList.toggle("dark", mode === "dark" || (mode === "auto" && mq.matches));
    apply();
    mq.addEventListener("change", apply);
    try { localStorage.setItem("batik-theme", mode); } catch {}
    return () => mq.removeEventListener("change", apply);
  }, [mode]);
  return [mode, setMode];
}

/* ───────────── Animasi laut (ombak, gelembung, manta, ikan, pulau) ───────────── */
const NS = "http://www.w3.org/2000/svg";
const W = 268;
const R = (a, b) => a + Math.random() * (b - a);
const wave = (amp, crest) => {
  let d = `M-134 0Q-100.5 ${-amp * 2} -67 0`;
  for (let i = 0; i < 7; i++) d += `T${i * 67} 0`;
  return crest ? d : d + "V3000H-134Z";
};
const mk = (tag, at, par) => {
  const e = document.createElementNS(NS, tag);
  for (const k in at) e.setAttribute(k, at[k]);
  par.appendChild(e);
  return e;
};
const leaves = () =>
  [-165, -130, -95, -60, -25, 15]
    .map((a) => `<path d="M0 0C8-9 22-9 30-1C20-5 10-4 0 0Z" transform="rotate(${a})" style="fill:var(--palm)"/>`)
    .join("");

function mountSea(svg) {
  const still = matchMedia("(prefers-reduced-motion: reduce)").matches;
  let raf = 0, tok = 0;

  const osc = (e, a, b, d, origin) => {
    if (still) return;
    e.style.transformOrigin = origin || "0 0";
    e.animate([{ transform: `rotate(${a})` }, { transform: `rotate(${b})` }],
      { duration: d, iterations: Infinity, direction: "alternate", easing: "ease-in-out" });
  };

  function build() {
    cancelAnimationFrame(raf);
    const my = ++tok;
    const h = svg.clientHeight || 800;
    const lvl = Math.max(h * 0.64, 430);
    svg.setAttribute("viewBox", `0 0 ${W} ${h}`);

    const L = [
      ["back", 12, -18, 0.4, 11000, 134, 5200],
      ["mid", 10, -8, 0.62, 7000, -134, 4300],
      ["front", 8, 0, 0.92, 5000, 134, 3600],
    ];
    const layer = (l) =>
      `<g id="${l[0]}" transform="translate(0 ${lvl + l[2]})"><g id="${l[0]}i"><path d="${wave(l[1])}" fill="url(#wg)" opacity="${l[3]}"/>` +
      (l[0] === "mid"
        ? `<path d="${wave(l[1], 1)}" fill="none" style="stroke:var(--foam)" stroke-width="2" stroke-linecap="round" opacity=".85"/>`
        : "") + `</g></g>`;

    svg.innerHTML =
      `<defs><linearGradient id="wg" gradientUnits="userSpaceOnUse" x1="0" y1="0" x2="0" y2="420"><stop offset="0" style="stop-color:var(--w1)"/><stop offset="1" style="stop-color:var(--w2)"/></linearGradient></defs>` +
      layer(L[0]) +
      `<g id="isl" transform="translate(194 ${lvl - 4})"><path d="M-56 18C-48 -4 -24 -18 0 -18C24 -18 48 -4 56 18Z" style="fill:var(--sand)"/>` +
      `<path d="M-30 -2C-18 -14 -4 -17 8 -14C22 -17 34 -8 42 2C22 -4 0 -4 -30 -2Z" style="fill:var(--palm)" opacity=".8"/>` +
      `<g transform="translate(16 -14)"><g class="pl"><path d="M0 0C3 -14 8 -26 14 -36" fill="none" style="stroke:var(--trunk)" stroke-width="3" stroke-linecap="round"/><g transform="translate(14 -36)">${leaves()}<circle cx="1" cy="3" r="2.2" style="fill:var(--trunk)"/></g></g></g>` +
      `<g transform="translate(-20 -10) scale(.65)"><g class="pl"><path d="M0 0C-2 -12 -6 -22 -12 -30" fill="none" style="stroke:var(--trunk)" stroke-width="3" stroke-linecap="round"/><g transform="translate(-12 -30)">${leaves()}</g></g></g></g>` +
      layer(L[1]) + layer(L[2]) + `<g id="cre"></g><g id="bub"></g>`;

    const q = (id) => svg.querySelector("#" + id);
    if (!still) {
      L.forEach((l) => {
        q(l[0] + "i").animate([{ transform: "translateX(0)" }, { transform: `translateX(${l[5]}px)` }],
          { duration: l[4], iterations: Infinity });
        q(l[0]).animate(
          [{ transform: `translate(0px,${lvl + l[2]}px)` }, { transform: `translate(0px,${lvl + l[2] + 9}px)` }],
          { duration: l[6], iterations: Infinity, direction: "alternate", easing: "ease-in-out" });
      });
      q("isl").animate(
        [{ transform: `translate(194px,${lvl - 6}px)` }, { transform: `translate(194px,${lvl + 3}px)` }],
        { duration: 4300, iterations: Infinity, direction: "alternate", easing: "ease-in-out" });
      svg.querySelectorAll(".pl").forEach((p, i) => osc(p, "-3deg", "4deg", 2600 + i * 700));
    }

    /* manta & ikan berenang acak */
    const cre = q("cre"), ag = [];
    const x0 = 18, x1 = W - 18, y0 = lvl + 36, y1 = Math.max(h - 34, y0 + 1);
    const manta = (sc, op, sp) => {
      const e = mk("g", { opacity: op }, cre);
      e.innerHTML =
        `<g class="tl"><path d="M-30 2C-40 3-48 8-62 6C-48 0-40 -1-30 -1Z" style="fill:var(--cr)"/></g>` +
        `<g class="w2"><path d="M16 4C4 14-12 20-28 20C-16 12-8 8-4 2Z" style="fill:var(--cr)" opacity=".6"/></g>` +
        `<path d="M36 0C28-7 6-9-10-3C-18-1-26 0-32 2C-26 3-18 3-10 5C6 9 28 7 36 0Z" style="fill:var(--cr)"/>` +
        `<path d="M36 0l5-3-1 5zM34 2l5 2-4 1z" style="fill:var(--cr)"/>` +
        `<g class="w1"><path d="M16-4C4-16-16-24-34-22C-20-14-8-8-4-2Z" style="fill:var(--cr)"/></g><circle cx="27" cy="-2" r="1.2" style="fill:var(--foam)"/>`;
      osc(e.querySelector(".w1"), "-12deg", "16deg", 1300, "12px -4px");
      osc(e.querySelector(".w2"), "10deg", "-14deg", 1300, "12px 4px");
      osc(e.querySelector(".tl"), "-8deg", "8deg", 1500, "-30px 0px");
      ag.push({ e, s: sc, vr: [sp * 0.6, sp] });
    };
    const fish = (red, sc) => {
      const e = mk("g", {}, cre), c = red ? "var(--acc)" : "var(--foam)";
      e.innerHTML =
        `<g class="tl"><path d="M-8 0L-18-6L-18 6Z" style="fill:${c}"/></g><path d="M-9 0C-5-6 5-6 12 0C5 6-5 6-9 0Z" style="fill:${c}"/>` +
        `<path d="M2-5L2 5" style="stroke:${red ? "var(--foam)" : "var(--acc)"}" stroke-width="2"/><circle cx="8" cy="-1.5" r="1" style="fill:var(--ink)"/>`;
      osc(e.querySelector(".tl"), "-16deg", "16deg", R(260, 380), "-8px 0px");
      ag.push({ e, s: sc, vr: [34, 66] });
    };
    manta(0.55, 0.55, 18);
    manta(0.9, 0.9, 24);
    for (let i = 0; i < 6; i++) fish(i % 3 !== 2, R(0.85, 1.15));

    const pick = (a) => {
      for (let t = 0; t < 6; t++) {
        a.tx = R(x0, x1); a.ty = R(y0, y1);
        if (Math.hypot(a.tx - a.x, a.ty - a.y) > 90) break;
      }
      a.sp = R(a.vr[0], a.vr[1]);
    };
    const draw = (a) => {
      const th = Math.max(-26, Math.min(26, Math.atan2(a.vy, Math.max(Math.abs(a.vx), 8)) * 57.3));
      a.e.setAttribute("transform", `translate(${a.x} ${a.y}) scale(${a.sx * a.s} ${a.s}) rotate(${th})`);
    };
    ag.forEach((a) => { Object.assign(a, { x: R(x0, x1), y: R(y0, y1), vx: R(-20, 20), vy: 0, f: 1, sx: 1 }); pick(a); });

    if (still) ag.forEach(draw);
    else {
      let last = performance.now();
      const frame = (t) => {
        if (my !== tok) return;
        const dt = Math.min((t - last) / 1000, 0.05);
        last = t;
        ag.forEach((a) => {
          const dx = a.tx - a.x, dy = a.ty - a.y, d = Math.hypot(dx, dy) || 1;
          if (d < 26) pick(a);
          const k = Math.min(1, dt * 1.1);
          a.vx += ((dx / d) * a.sp - a.vx) * k;
          a.vy += ((dy / d) * a.sp - a.vy) * k;
          a.x = Math.max(x0, Math.min(x1, a.x + a.vx * dt));
          a.y = Math.max(y0, Math.min(y1, a.y + a.vy * dt));
          if (Math.abs(a.vx) > 4) a.f = a.vx > 0 ? 1 : -1;
          a.sx += (a.f - a.sx) * Math.min(1, dt * 4);
          draw(a);
        });
        raf = requestAnimationFrame(frame);
      };
      raf = requestAnimationFrame(frame);
    }

    /* gelembung (bunga batik di gelembung besar) */
    const b = q("bub");
    for (let i = 0; i < 16; i++) {
      const r = 3 + R(0, 10), x = R(14, W - 14), top = lvl - 50 - R(0, 40), dur = R(6000, 12000), sw = R(8, 22);
      const e = mk("g", {}, b);
      mk("circle", { r, style: "fill:var(--foam);stroke:var(--foam)", "fill-opacity": 0.13, "stroke-opacity": 0.8, "stroke-width": 1.2 }, e);
      mk("path", { d: `M${-r * 0.55} ${-r * 0.1}A${r * 0.6} ${r * 0.6} 0 0 1 ${-r * 0.1} ${-r * 0.58}`, fill: "none", style: "stroke:var(--foam)", "stroke-width": 1.3, "stroke-linecap": "round", opacity: 0.9 }, e);
      if (r >= 8) {
        const f = mk("g", { opacity: 0.75 }, e);
        for (let k = 0; k < 4; k++)
          mk("ellipse", { cy: -r * 0.3, rx: r * 0.13, ry: r * 0.28, fill: "none", style: "stroke:var(--foam)", "stroke-width": 0.9, transform: `rotate(${k * 90})` }, f);
        mk("circle", { r: 1.1, style: "fill:var(--foam)" }, f);
      }
      const yb = h + 20;
      const tr = (t, dx, sc) => `translate(${x + dx}px,${yb + (top - yb) * t}px) scale(${sc})`;
      if (still) { e.style.transform = tr(Math.random(), 0, 1); continue; }
      e.animate([
        { transform: tr(0, 0, 0.6), opacity: 0, offset: 0 },
        { transform: tr(0.1, sw * 0.3, 0.8), opacity: 0.95, offset: 0.1 },
        { transform: tr(0.35, sw, 0.95), opacity: 0.95, offset: 0.35 },
        { transform: tr(0.65, -sw, 1), opacity: 0.95, offset: 0.65 },
        { transform: tr(0.93, sw * 0.4, 1), opacity: 0.95, offset: 0.93 },
        { transform: tr(1, 0, 1.35), opacity: 0, offset: 1 },
      ], { duration: dur, delay: -Math.random() * dur, iterations: Infinity, easing: "linear" });
    }
  }

  build();
  let tm;
  const ro = new ResizeObserver(() => { clearTimeout(tm); tm = setTimeout(build, 200); });
  ro.observe(svg);
  return () => { tok++; cancelAnimationFrame(raf); clearTimeout(tm); ro.disconnect(); svg.innerHTML = ""; };
}

function Sea() {
  const ref = useRef(null);
  useEffect(() => mountSea(ref.current), []);
  return <svg ref={ref} aria-hidden="true" className="pointer-events-none absolute inset-0 h-full w-full [filter:var(--glow)]" />;
}

/* ───────────── Sidebar ───────────── */
const DEFAULT_ITEMS = ["Dashboard", "Pesanan", "Produk", "Pelanggan", "Laporan", "Pengaturan"];

export default function BatikSidebar({ items = DEFAULT_ITEMS, active = items[0], onSelect, brand = "Nusantara Admin" }) {
  const [mode, setMode] = useTheme();
  return (
    <aside className="relative flex w-[268px] shrink-0 flex-col overflow-hidden border-r border-[var(--line)] bg-[var(--sb)] transition-colors duration-500 max-sm:w-52">
      <svg aria-hidden="true" className="absolute inset-0 h-full w-full text-[var(--w2)] opacity-[.06]">
        <defs>
          <pattern id="kawung" width="44" height="44" patternUnits="userSpaceOnUse">
            <g fill="none" stroke="currentColor" strokeWidth="1.2">
              <ellipse cx="22" cy="11" rx="6" ry="10" /><ellipse cx="22" cy="33" rx="6" ry="10" />
              <ellipse cx="11" cy="22" rx="10" ry="6" /><ellipse cx="33" cy="22" rx="10" ry="6" />
            </g>
            <circle cx="22" cy="22" r="2" fill="currentColor" />
          </pattern>
        </defs>
        <rect width="100%" height="100%" fill="url(#kawung)" />
      </svg>

      <Sea />

      <div className="relative z-10 flex items-center gap-2.5 px-[22px] pb-3.5 pt-[22px] font-bold tracking-wide text-[var(--ink)]">
        <div className="grid size-[34px] place-items-center rounded-[10px] bg-[var(--acc)] text-lg text-white">✿</div>
        {brand}
      </div>

      <nav className="relative z-10 flex flex-col gap-1 px-3 py-2">
        {items.map((label) => {
          const on = label === active;
          return (
            <a
              key={label}
              href="#"
              aria-current={on ? "page" : undefined}
              onClick={(e) => { e.preventDefault(); onSelect?.(label); }}
              className={
                "group flex items-center gap-3 rounded-xl px-3.5 py-[11px] text-sm text-[var(--ink)] transition duration-200 hover:translate-x-1 hover:bg-[var(--soft)] focus-visible:outline-2 focus-visible:outline-[var(--acc)] " +
                (on ? "bg-[var(--soft)] font-semibold shadow-[inset_3px_0_0_var(--acc)]" : "")
              }
            >
              <span className={"size-2 shrink-0 rounded-full border-2 border-[var(--acc)] transition-colors group-hover:bg-[var(--acc)] " + (on ? "bg-[var(--acc)]" : "")} />
              {label}
            </a>
          );
        })}
      </nav>

      <div className="relative z-10 mt-auto p-3.5">
        <div role="group" aria-label="Tema" className="flex gap-0.5 rounded-full border border-[var(--line)] bg-[var(--sb)] p-1">
          {[["light", "☀ Terang"], ["dark", "☾ Gelap"], ["auto", "Auto"]].map(([m, t]) => (
            <button
              key={m}
              onClick={() => setMode(m)}
              aria-pressed={mode === m}
              className={
                "flex-1 cursor-pointer rounded-full px-1 py-2 text-xs transition-colors " +
                (mode === m ? "bg-[var(--acc)] text-white" : "text-[var(--mut)]")
              }
            >
              {t}
            </button>
          ))}
        </div>
      </div>
    </aside>
  );
}
