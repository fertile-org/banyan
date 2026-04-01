# Design System — Banyan

## Product Context
- **What this is:** Container orchestration platform that bridges docker compose and production
- **Who it's for:** Small to medium teams (2-30 engineers) who know Docker Compose, don't have dedicated DevOps
- **Space/industry:** DevOps, container orchestration, infrastructure management
- **Project type:** Operational dashboard (web app) + CLI TUI + documentation site

## Aesthetic Direction
- **Direction:** Industrial/Utilitarian
- **Decoration level:** Minimal — typography, spacing, and color do the work. No gradients, no decorative elements. Borders are thin and purposeful.
- **Mood:** Terminal-native. The web dashboard should feel like a polished extension of the CLI TUI, not a different product. Function-first, data-dense, monospace accents. Built for engineers who live in terminals.
- **Reference sites:** Researched Argo CD, Portainer, Grafana, Datadog, Coolify. Banyan differentiates with opinionated simplicity (no drag-and-drop widget building) and terminal aesthetic (monospace for data).

## Typography
- **Display/Nav/Body:** Geist — Vercel's open geometric sans-serif. Clean at small sizes, pairs naturally with its mono variant. Not Inter (overused in DevOps tools).
- **Data/Tables/Logs/Code:** Geist Mono — same family, built-in tabular-nums. Terminal-native feel for container names, IDs, timestamps, log output.
- **Loading:** Google Fonts CDN (`family=Geist:wght@400;500;600;700&family=Geist+Mono:wght@400;500;600`)
- **Scale:**
  - Page title: 20px / 700
  - Section title: 16px / 700
  - Panel title: 13px / 600
  - Body: 13px / 400
  - Label/Caption: 12px / 600
  - Muted/Sub: 11px / 400
  - Mono data: 12px / 400

## Icons
- **Pack:** Lucide (https://lucide.dev) — the icon library used by shadcn/ui natively (`lucide-react`)
- **Size:** 16px for inline/sidebar, 14px for buttons/actions, 12px for stat card sub-text
- **Style:** Stroke icons only (consistent with Lucide defaults), color inherits from text
- **Key mappings:**
  - Overview: `layout-dashboard`
  - Agents: `server`
  - Deployments: `rocket`
  - Containers: `container`
  - Engine: `cpu`
  - Events: `activity`
  - Logs: `scroll-text`
  - Secrets: `key-round`
  - Settings: `settings`
  - Status healthy: `check-circle`
  - Status warning: `alert-triangle`
  - Status error: `x-circle`
  - Actions: `more-horizontal`
  - Filter: `filter`
  - Export: `download`
  - Refresh: `refresh-cw`
  - Delete/Teardown: `trash-2`
  - Scale: `scaling`
  - Health: `heart-pulse`
  - Theme dark: `moon`
  - Theme light: `sun`

## Color
- **Approach:** Restrained — one primary accent + neutrals. Color is rare and meaningful (status indicators, actions).
- **Dark mode (default):**
  - Background: `#0F1117`
  - Surface (cards, panels): `#161B22`
  - Surface hover: `#1C2129`
  - Border: `#21262D`
  - Border accent: `#30363D`
  - Text: `#E6EDF3`
  - Text secondary: `#8B949E`
  - Text muted: `#6E7681`
  - Primary: `#7CB342` (Banyan green, matches TUI)
  - Primary hover: `#8BC34A`
  - Primary dim: `rgba(124,179,66,0.15)`
- **Light mode:**
  - Background: `#FFFFFF`
  - Surface: `#F6F8FA`
  - Surface hover: `#EEF1F4`
  - Border: `#D0D7DE`
  - Border accent: `#B8BFC7`
  - Text: `#1F2328`
  - Text secondary: `#656D76`
  - Text muted: `#8C959F`
  - Primary: `#558B2F`
  - Primary hover: `#689F38`
  - Primary dim: `rgba(85,139,47,0.1)`
- **Semantic (both modes):**
  - Green (success/running): dark `#3FB950` / light `#1A7F37`
  - Yellow (warning/pending): dark `#D29922` / light `#9A6700`
  - Red (error/failed): dark `#F85149` / light `#CF222E`
  - Blue (info/link): dark `#58A6FF` / light `#0969DA`
  - Each semantic color has a dim variant at 15% opacity (dark) or 10% opacity (light) for backgrounds

## Spacing
- **Base unit:** 4px
- **Density:** Comfortable — not as cramped as Grafana, not as spacious as marketing sites
- **Scale:** 2xs(2px) xs(4px) sm(8px) md(16px) lg(24px) xl(32px) 2xl(48px) 3xl(64px)
- **Common patterns:**
  - Card padding: 16px
  - Panel header padding: 12px 16px
  - Table cell padding: 10px 16px
  - Section gap: 24px
  - Card grid gap: 16px
  - Sidebar item padding: 8px 16px

## Layout
- **Approach:** Sidebar + opinionated content — no drag-and-drop widget building
- **Navigation:** Left sidebar (230px), collapsible to icon rail. Sections: Cluster (Overview, Agents, Deployments, Containers, Engine), Operations (Events, Logs, Secrets), Settings.
- **Content area:** Full-width with 24px padding. Stat cards in a grid row, then panels with tables.
- **Max content width:** None (fills available space, data tables benefit from width)
- **Border radius:** sm: 4px, md: 6px, lg: 8px (minimal, not bubbly)
- **Stat cards:** 5-column grid (Engines, Agents, Deployments, Containers, Tasks) — each shows `connected/total` with a status sub-text

## Motion
- **Approach:** Minimal-functional — only transitions that aid comprehension
- **Easing:** ease-out for enter, ease-in for exit
- **Duration:** hover states 150ms, panel open/close 200ms
- **No:** entrance animations, scroll effects, loading spinners beyond simple indicators. This is an ops tool.

## Component Patterns
- **Status badges:** Pill-shaped (`border-radius: 10px`), colored dot + text, dim background matching status color
- **Tables:** Full-width, monospace for data columns (container names, IDs, ports, memory), hover highlight on rows, action menu via `...` icon button
- **Buttons:** Primary (green fill), Secondary (border only), Danger (red text + border), Ghost (no border). All include icons.
- **Alerts:** Full-width, colored border + dim background + icon. Success/Warning/Error/Info.
- **Log pane:** Monospace, dark background (`--bg`), colored log levels (blue INFO, yellow WARN, red ERROR), timestamps in muted color
- **Forms:** Monospace inputs on dark background, green focus ring with dim shadow

## Anti-Patterns (never use)
- Purple/violet gradients
- 3-column feature grid with icons in colored circles
- Centered everything with uniform spacing
- Large border-radius on all elements (keep it tight: 4-8px)
- Gradient buttons
- Drag-and-drop dashboard building (the dashboard IS the deployment)
- Custom widget configuration (zero-config, auto-generated from deployment state)

## Decisions Log
| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-03-31 | Initial design system created | Created by /design-consultation based on competitive research (Argo CD, Portainer, Grafana, Datadog, Coolify) and Banyan's existing TUI identity |
| 2026-03-31 | Geist + Geist Mono typography | One coherent font family for sans + mono. Not Inter (overused). Pairs naturally, designed together by Vercel. |
| 2026-03-31 | Lucide icons | Native to shadcn/ui (our component library). Consistent stroke style. |
| 2026-03-31 | Dark-first, no customizable layout | Matches terminal-native audience. Zero-config dashboard aligned with Banyan's "sensible defaults" principle. |
| 2026-03-31 | 5 stat cards including Engines | Multi-engine HA is shipped (M7). Engine card shows connected/total like all other resources. |
