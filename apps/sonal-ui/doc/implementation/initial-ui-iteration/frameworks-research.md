# Svelte UI component ecosystems for a Vite-only SPA (2025–2026)

## Executive summary

For a **Svelte 5 + Vite static SPA** (no SvelteKit), the notes support a shortlist of **well-maintained, actively released** options: **Skeleton** (`@skeletonlabs/skeleton-svelte`), **shadcn-svelte** with **Bits UI**, **Flowbite Svelte**, and **IBM Carbon Components Svelte**. All four show **recent 2026 releases** and **Svelte 5–oriented** contracts (peer ranges, docs, or examples). **Skeleton** and **Flowbite Svelte** are **Tailwind-centric** and mutually **incompatible** in one app: Skeleton’s docs state it **cannot** integrate with Flowbite Svelte (or Daisy UI) due to overlapping Tailwind changes. **shadcn-svelte** is a **copy-paste / CLI + registry** model with **Bits UI** as the headless layer—not a single npm UI package—and documents **plain Vite** setup alongside SvelteKit. **Flowbite Svelte** couples **Tailwind v4**–style config (`@source` scanning `flowbite-svelte/dist`) and explicit **non–SvelteKit** path adjustment prose. **Carbon** is a **full IBM design system** port with **precompiled themes and SCSS**, official **Vite** example without Kit, and **enterprise** look-and-feel vs utility-first stacks. **Primary direction:** choose **Skeleton** or **shadcn-svelte + Bits** for **Tailwind-native** product UI with different lock-in profiles (packaged design system vs owned source); choose **Flowbite Svelte** if the **Flowbite** visual system and Tailwind v4 pipeline are fixed requirements **and** Skeleton is ruled out; choose **Carbon** when **Carbon compliance**, data-dense UI, and IBM-aligned branding outweigh Tailwind flexibility.

## Key findings

1. **Skeleton** documents **Svelte 5** as the minimum on the Vite path, declares **`peerDependencies.svelte: ^5.29.0`**, and uses runes-style examples (`$state`, `$props`, snippets) in fundamentals ([Skeleton Vite + Svelte](https://www.skeleton.dev/docs/svelte/get-started/installation/vite-svelte), [package.json](https://raw.githubusercontent.com/skeletonlabs/skeleton/main/packages/skeleton-svelte/package.json), [Fundamentals](https://www.skeleton.dev/docs/svelte/get-started/fundamentals)).
2. **Skeleton** explicitly **cannot** integrate **Flowbite Svelte** or **Daisy UI** alongside Skeleton because those kits overlap Tailwind in ways that conflict with Skeleton’s core features ([Installation](https://www.skeleton.dev/docs/svelte/get-started/installation.md)).
3. **shadcn-svelte** distributes **multi-file components** per component folder, recommends the **CLI** as optimal, and ties **Svelte 5** migration to **`bits-ui ^1`** plus registry and Tailwind updates ([Installation](https://www.shadcn-svelte.com/docs/installation), [Svelte 5 migration](https://www.shadcn-svelte.com/docs/migration/svelte-5)).
4. **shadcn-svelte** documents **plain Vite**: Tailwind via `sv add`, path aliases, `shadcn-svelte init`, and `add` for components ([Vite installation](https://www.shadcn-svelte.com/docs/installation/vite)).
5. **Flowbite Svelte** declares **`svelte` peer `^5.40.0`** and **`tailwindcss` `^4.1.4`**, depends on **`flowbite`**, and documents **“Other Project Types”** for non-SvelteKit Vite apps with CSS path adjustments ([package.json](https://raw.githubusercontent.com/themesberg/flowbite-svelte/main/package.json), [Quickstart](https://flowbite-svelte.com/docs/pages/quickstart)).
6. **Carbon Components Svelte** ships **frequent** releases (e.g. **v0.105.0** March 2026), aligns APIs with **Svelte 5 snippets** in release notes (e.g. DataTable slot rename in v0.102.0), and provides an official **`examples/vite`** stack without SvelteKit ([Releases](https://github.com/carbon-design-system/carbon-components-svelte/releases), [Vite example package.json](https://raw.githubusercontent.com/carbon-design-system/carbon-components-svelte/master/examples/vite/package.json), [README](https://github.com/carbon-design-system/carbon-components-svelte/blob/master/README.md)).
7. **Discovery** also surfaced **Melt UI** (builders, pre-1.0 stability caveat) and **Grail UI** as headless/primitives options; they were not deep-tasked to the same depth as the four candidates ([discovery-seed](notes/discovery-seed.md)).

## Detailed sections (plan questions)

### Which mature Svelte component options exist (2025–2026)?

Task notes establish **Skeleton**, **shadcn-svelte + Bits UI**, **Flowbite Svelte**, and **IBM Carbon Components Svelte** as primary contenders, each with official docs and GitHub/npm presence. Discovery additionally listed **Melt UI**, **Grail UI**, and cross-framework Skeleton marketing ([discovery-seed](notes/discovery-seed.md)).

### Maintenance, Svelte 5, and fit for plain Vite SPA

- **Skeleton:** Per-package releases (e.g. `@skeletonlabs/skeleton-svelte@4.13.0` March 2026); **Vite + Svelte** guide is the SPA-oriented path with Tailwind 4 and theme imports ([T2](notes/task-T2-skeleton-svelte-ui.md)).
- **shadcn-svelte:** Patch releases through **1.2.x** (March 2026); migration doc for **Svelte 5** + **bits-ui ^1**; **Vite** install is first-class ([T3](notes/task-T3-shadcn-svelte-bits-ui.md)).
- **Flowbite Svelte:** **v1.33.0** latest (March 2026 per notes), **v2** prerelease line parallel; official **Vite + Svelte** quickstart; explicit note for **non-SvelteKit** projects to adjust CSS/`@source` paths ([T4](notes/task-T4-flowbite-svelte.md)).
- **Carbon:** **v0.105.0** (March 2026) and same-week adjacent versions; **examples/vite** uses Vite + Svelte 5 without Kit ([T5](notes/task-T5-carbon-components-svelte.md)).

### Trade-offs: design system vs headless vs copy-paste; bundle and theming; accessibility

- **Skeleton:** **Adaptive design system** on Tailwind; complements **headless** libs (Bits, Melt, Radix, Zag) but **not** Flowbite Svelte/Daisy ([T2](notes/task-T2-skeleton-svelte-ui.md)).
- **shadcn-svelte + Bits:** **Copy-paste / CLI-owned source**; **Bits** provides **headless** primitives; Tailwind theme variables tied to Bits (e.g. accordion height CSS vars) ([T3](notes/task-T3-shadcn-svelte-bits-ui.md)). Bits positioning in discovery: a11y, unstyled primitives ([discovery-seed](notes/discovery-seed.md)).
- **Flowbite Svelte:** **Tailwind utilities + Flowbite** shared design language; tight **Tailwind v4** scanning via `@source` ([T4](notes/task-T4-flowbite-svelte.md)).
- **Carbon:** **Precompiled theme CSS** and optional **SCSS** per component; **`optimizeCss`** and **`optimizeImports`** for smaller builds; **IBM Carbon** visuals, not utility-first Tailwind composition ([T5](notes/task-T5-carbon-components-svelte.md)).

### Risk: lock-in, breaking changes, migration

- **Skeleton vs Flowbite:** **Mutual exclusion** documented if considering both—reduces “try both” risk only by forcing a single Tailwind-stack choice ([T2](notes/task-T2-skeleton-svelte-ui.md)).
- **shadcn-svelte:** Upgrades use **`add --overwrite`** (replace files); **bits-ui** major bump coordinated with Svelte 5 migration; optional **npm aliasing** for gradual Bits migration ([T3](notes/task-T3-shadcn-svelte-bits-ui.md)).
- **Flowbite Svelte:** **Stable v1** vs **v2 next** line implies future **major** migration work ([T4](notes/task-T4-flowbite-svelte.md)).
- **Carbon:** **Semantic** 0.x releases with **documented** breaking changes (e.g. snippet-related slot renames) ([T5](notes/task-T5-carbon-components-svelte.md)).
- **Melt UI (discovery):** **Pre-1.0** semver behavior noted in upstream docs—higher churn risk if adopted without task-level verification ([discovery-seed](notes/discovery-seed.md)).

### Practical integration: TypeScript, tree-shaking, Vitest + Testing Library

- **Skeleton:** TypeScript in Vite template; MIT license ([T2](notes/task-T2-skeleton-svelte-ui.md)).
- **shadcn-svelte:** Multi-file components **tree-shaken** by Rollup per docs; **`utils.ts`** / `cn` patterns in migration ([T3](notes/task-T3-shadcn-svelte-bits-ui.md)). Task notes **did not** validate Vitest + Testing Library friction empirically—**inference:** copy-paste layout is standard Svelte testing territory.
- **Flowbite Svelte:** Repo scripts include **Vitest** and **Playwright** ([T4](notes/task-T4-flowbite-svelte.md)).
- **Carbon:** **sveld**-generated types; generics improvements in releases; **`optimizeImports`** for direct `.svelte` paths ([T5](notes/task-T5-carbon-components-svelte.md)).

### Recommendation: 1–2 primary picks and when to prefer each

1. **Skeleton** — Prefer when you want a **maintained, packaged** Tailwind design system with **official Vite SPA** docs and **Svelte 5** as the baseline, and you may combine **headless** primitives (Bits/Melt) where needed. **Do not** pair with **Flowbite Svelte** in the same app ([T2](notes/task-T2-skeleton-svelte-ui.md)).
2. **shadcn-svelte + Bits UI** — Prefer when **owning source** in-repo, **CLI/registry** workflow, and **shadcn**-aligned patterns matter more than a single versioned npm UI package; **Vite** is documented ([T3](notes/task-T3-shadcn-svelte-bits-ui.md)).

**Secondary fits (from notes):** **Flowbite Svelte** if **Flowbite**’s design and Tailwind v4 pipeline are non-negotiable **and** Skeleton is excluded **or** Flowbite is chosen instead of Skeleton ([T4](notes/task-T4-flowbite-svelte.md)). **Carbon** when **enterprise IBM UX**, theming via Carbon CSS, and **data-heavy** patterns dominate ([T5](notes/task-T5-carbon-components-svelte.md)).

## Comparative matrix (grounded in task notes only)

| Candidate | Maintenance (notes) | Svelte 5 | Vite SPA fit (plain, no Kit) | Model (design system vs headless vs copy-paste) |
| --- | --- | --- | --- | --- |
| **Skeleton** | Frequent releases; **4.13.0** Mar 2026; prior **4.12.x** weeks apart ([T2](notes/task-T2-skeleton-svelte-ui.md)) | Min Svelte 5 on Vite path; peer **`^5.29.0`**; runes in docs ([T2](notes/task-T2-skeleton-svelte-ui.md)) | **Vite + Svelte** guide: `create vite`, Tailwind, `app.css`, `index.html` theme ([T2](notes/task-T2-skeleton-svelte-ui.md)) | **Design system** on Tailwind; complements **headless** libs; **not** with Flowbite Svelte/Daisy ([T2](notes/task-T2-skeleton-svelte-ui.md)) |
| **shadcn-svelte + Bits** | **1.2.x** patches Mar 2026; registry/schema churn documented ([T3](notes/task-T3-shadcn-svelte-bits-ui.md)) | Migration doc; **`bits-ui ^1`** ([T3](notes/task-T3-shadcn-svelte-bits-ui.md)) | **Vite** install: `sv add tailwindcss`, aliases, `init`/`add` ([T3](notes/task-T3-shadcn-svelte-bits-ui.md)) | **Copy-paste / CLI**; **Bits** = **headless** layer ([T3](notes/task-T3-shadcn-svelte-bits-ui.md)) |
| **Flowbite Svelte** | **v1.33.0** Mar 2026; **v2** prereleases; many historical releases ([T4](notes/task-T4-flowbite-svelte.md)) | Peer **`^5.40.0`** ([T4](notes/task-T4-flowbite-svelte.md)) | **Vite + Svelte** quickstart; **“Other Project Types”** for path fixes ([T4](notes/task-T4-flowbite-svelte.md)) | **Tailwind + Flowbite** component kit (npm package) ([T4](notes/task-T4-flowbite-svelte.md)) |
| **Carbon Components Svelte** | **v0.105.0** Mar 2026; rapid 0.10x line ([T5](notes/task-T5-carbon-components-svelte.md)) | Dev **Svelte 5**; snippet-related API changes in releases ([T5](notes/task-T5-carbon-components-svelte.md)) | **`examples/vite`** without SvelteKit ([T5](notes/task-T5-carbon-components-svelte.md)) | **IBM design system** (CSS/SCSS themes), not Tailwind-first ([T5](notes/task-T5-carbon-components-svelte.md)) |

## Source list

- https://www.skeleton.dev/docs/svelte/get-started/installation — Skeleton installation hub; unsupported libs vs headless. Accessed 2026-03-30 (task notes).
- https://www.skeleton.dev/docs/svelte/get-started/installation/vite-svelte — Vite + Svelte requirements and steps. Accessed 2026-03-30.
- https://www.skeleton.dev/docs/svelte/get-started/fundamentals — Runes-style examples. Accessed 2026-03-30.
- https://raw.githubusercontent.com/skeletonlabs/skeleton/main/packages/skeleton-svelte/package.json — Peer Svelte range, MIT. Accessed 2026-03-30.
- https://github.com/skeletonlabs/skeleton/blob/main/LICENSE — Repository MIT license. Accessed 2026-03-30.
- https://github.com/skeletonlabs/skeleton/releases — Monorepo release index. Accessed 2026-03-30.
- https://github.com/skeletonlabs/skeleton/releases/tag/@skeletonlabs/skeleton-svelte@4.13.0 — Package release notes. Accessed 2026-03-30.
- https://www.shadcn-svelte.com/docs/installation — Multi-file components, CLI, tree-shaking. Accessed 2026-03-30.
- https://www.shadcn-svelte.com/docs/installation/vite — Plain Vite setup. Accessed 2026-03-30.
- https://www.shadcn-svelte.com/docs/cli — CLI init/add/registry. Accessed 2026-03-30.
- https://www.shadcn-svelte.com/docs/registry — Custom registries (experimental). Accessed 2026-03-30.
- https://www.shadcn-svelte.com/docs/migration/svelte-5 — Svelte 5 migration, bits-ui ^1. Accessed 2026-03-30.
- https://github.com/huntabyte/shadcn-svelte/releases — Release cadence. Accessed 2026-03-30.
- https://flowbite-svelte.com/docs/pages/quickstart — Quickstart, non-SvelteKit note. Accessed 2026-03-30.
- https://flowbite-svelte.com/docs/pages/introduction — Tailwind v4 `@source` coupling. Accessed 2026-03-30.
- https://github.com/themesberg/flowbite-svelte — Repo stats, positioning. Accessed 2026-03-30.
- https://github.com/themesberg/flowbite-svelte/releases — v1/v2 release lines. Accessed 2026-03-30.
- https://www.npmjs.com/package/flowbite-svelte — npm metadata. Accessed 2026-03-30.
- https://raw.githubusercontent.com/themesberg/flowbite-svelte/main/package.json — Peers and deps. Accessed 2026-03-30.
- https://svelte.carbondesignsystem.com/ — Carbon Svelte docs home. Accessed 2026-03-30.
- https://github.com/carbon-design-system/carbon-components-svelte/blob/master/README.md — Install, Vite, theming, optimizeCss. Accessed 2026-03-30.
- https://github.com/carbon-design-system/carbon-components-svelte/releases — v0.10x releases. Accessed 2026-03-30.
- https://www.npmjs.com/package/carbon-components-svelte — npm current version. Accessed 2026-03-30.
- https://raw.githubusercontent.com/carbon-design-system/carbon-components-svelte/master/package.json — Scripts, tooling. Accessed 2026-03-30.
- https://raw.githubusercontent.com/carbon-design-system/carbon-components-svelte/master/examples/vite/package.json — Plain Vite + Svelte 5. Accessed 2026-03-30.
- `notes/discovery-seed.md` — Discovery URLs, Melt/Grail, Skeleton–Flowbite conflict seed. Dated 2026-03-30.

## Gaps and limitations

- **Synthesis scope:** No new web research; claims are limited to **merged task notes**. Contradictions between git `main` and npm (e.g. Flowbite **main** `package.json` vs published **1.33.0**) were flagged in T4—pin versions from the **installed** artifact.
- **Incomplete deep coverage:** **Melt UI** and **Grail UI** appear in discovery but lack full task notes comparable to the four candidates.
- **Vitest + Testing Library:** No task note documented **empirical** friction for shadcn-svelte’s multi-file components—only architectural statements from installation docs.
- **Adoption metrics:** Stars/downloads cited in some tasks are **weak signals**; not all npm stats were transcribed (T4, T5).
- **Carbon:** Peer dependency range for Svelte was **not** fully confirmed from `package.json` snippet in T5—npm should be checked when pinning.
- **Firecrawl:** Search intermittently failed for some queries per discovery; some findings rely on **direct URL** access (documented in notes).

## Confidence

| Claim area | Level | Reason |
| --- | --- | --- |
| Skeleton vs Flowbite/Daisy incompatibility | **High** | Direct quoted constraint from official installation content ([T2](notes/task-T2-skeleton-svelte-ui.md)). |
| Svelte 5 peer ranges (Skeleton, Flowbite) | **High** | From published/raw `package.json` in notes. |
| shadcn-svelte distribution model + bits-ui ^1 | **High** | Official installation and migration docs ([T3](notes/task-T3-shadcn-svelte-bits-ui.md)). |
| Carbon Vite SPA support | **High** | Official README + `examples/vite` ([T5](notes/task-T5-carbon-components-svelte.md)). |
| Relative “best” for Sonal UI branding | **Medium** | Plan assumed **no** fixed design language; choice depends on product aesthetics not captured in notes. |
| Maintenance vs alternatives long-term | **Medium** | Recent releases are strong signals; no longitudinal issue-response metrics in notes. |

**Overall:** **Medium–high** for shortlist **existence** and **technical fit**; **medium** for **ranking** without product-specific design constraints.

## Suggested follow-ups

- Run a **minimal spike** in `apps/sonal-ui`: Skeleton **or** shadcn-svelte (one branch each) for one real screen; measure **DX**, **bundle**, and **test** ergonomics.
- If **Melt** or **Grail UI** matters: dedicated task note on **Svelte 5** compatibility from current **releases/README** (discovery flagged Melt pre-1.0).
- Confirm **Carbon** **peerDependencies** on target **npm** version and evaluate **IBM Telemetry** opt-out for your compliance needs ([T5](notes/task-T5-carbon-components-svelte.md)).
- Align **Tailwind major** (3 vs 4) across Sonal UI and the chosen stack using the **shadcn-svelte** migration sequence (Svelte 5 first, then Tailwind v4 doc) if upgrading ([T3](notes/task-T3-shadcn-svelte-bits-ui.md)).
