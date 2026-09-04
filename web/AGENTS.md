# Project Guidelines

To maintain a unified visual style and clean architecture, all agents and developers must adhere to the following guidelines for the `jman-ui` project.

## 0. Package Management

**ALWAYS** use `pnpm` for managing dependencies and running scripts. Do not use `npm` or `yarn`.

- Install dependencies: `pnpm install`
- Add a package: `pnpm add <package>`
- Run scripts: `pnpm <script>` (e.g., `pnpm dev`, `pnpm build`)

## 1. Modular CSS Architecture

Styles are modularized into specific files within `src/styles/`. Avoid a monolithic `style.css`.

- `variables.css`: Global CSS custom properties (colors, spacing, z-index).
- `base.css`: HTML resets, base element styles (input, select, a), and generic text utilities.
- `layout.css`: Layout containers (`view-container`, `grid-2-cols`), flex helpers, and app shell styles.
- `components.css`: Shared component styles (buttons, cards, tables, badges, tabs, accordions).
- `modals.css`: Standardized modal overlay and content styles.
- `auth.css`: Styles specific to the login and authentication flow.
- `toasts.css`: Toast notification styles and animations.

## 2. CSS Custom Properties (Variables)

**NEVER** use hardcoded hex, RGB, or HSL values in components or views. Always use CSS custom properties defined in `variables.css`.

- **Colors**: Use `var(--primary)`, `var(--text-main)`, `var(--bg-card)`, etc.
- **State**: Use status variables like `var(--error-text)`, `var(--badge-active-bg)`.
- **Dark Mode**: Support is handled automatically through these variables via `@media (prefers-color-scheme: dark)`.

## 3. Location of Styles

- **Views (`src/views/`)**: Must **NOT** contain `<style>` sections. Use global utility classes or component classes defined in the modular CSS files.
- **Components (`src/components/`)**: Scoped styles are permitted **ONLY** if the style is strictly unique to that specific component. If a style can be reused, move it to `src/styles/components.css`.
- **Inline Styles**: Avoid `style="..."` attributes. Use utility classes (e.g., `mt-4`, `flex-row`, `font-medium`) instead.

## 4. Standardized Components

Use existing class structures to ensure UI consistency:

- **Buttons**: Use `.btn` with modifiers like `.btn-primary`, `.btn-outline`, `.btn-text`, or `.btn-danger`.
- **Cards**: Use `.card` for standard containers and `.card-header` for titles with actions.
- **Tables**: Use `.data-table` inside a `.table-container`. Use `.clickable-row` for rows that navigate.
- **Badges**: Use `.status-badge` with state classes (`.active`, `.error`, `.info`, `.badge-sm`).
- **Modals**: Use `.modal-overlay` and `.modal-content.card`. Use `.modal-header` with `.modal-close`.

## 5. Responsive Utilities

- Use `.hide-mobile` and `.show-mobile` for simple toggling.
- Use responsive table utilities like `.hide-col-3-sm` to drop less important columns on small screens.
- Prefer CSS Grid (`.grid-2-cols`) and Flexbox over fixed widths.

## 6. CSS Nesting

The project uses modern CSS nesting. You can nest selectors inside their parents to improve readability:

```css
.card {
	padding: 20px;
	& h2 {
		margin-top: 0;
	}
}
```

## 7. Icons

**DO NOT** use inline SVGs in components or views. Use the `<AppIcon>` component.

- Icons live in `src/components/icons/` as template-only Vue SFCs (e.g., `IconSettings.vue`).
- `AppIcon` resolves the icon name to the corresponding component via a static import map in `src/components/AppIcon.vue`.
- Usage: `<AppIcon name="settings" size="18" />`.
- To add a new icon: create `src/components/icons/IconMyIcon.vue` with the SVG markup and register it in the `iconMap` in `AppIcon.vue`.

## 8. API Documentation

For information regarding backend API endpoints, payloads, and authentication, refer to the official API specification:

- [API Specification (API_SPECS.md)](https://github.com/JCO-Digital/jman/blob/main/docs/API_SPECS.md)

## 9. Code Design & Architecture

The project is built with **Vue 3**, **TypeScript**, and **Vite**.

### Vue Components

- Use the **Composition API** with `<script setup lang="ts">`.
- Keep components small and focused.
- Props should be clearly defined using `defineProps<{ ... }>()`.
- Emits should be clearly defined using `defineEmits<{ ... }>()`.

### State Management (Pinia)

- Use **Pinia** for global state management.
- Use the **Setup Store** syntax (passing a function as the second argument to `defineStore`).
- Organize stores by domain (e.g., `auth.ts`, `tasks.ts`, `data.ts`) in `src/stores/`.
- Access stores in components using `const store = useXStore()`.

### Directory Structure

- `src/components/`: Reusable UI components.
- `src/views/`: Main page components that correspond to routes.
- `src/stores/`: Pinia stores for state management.
- `src/utils/`: Helper functions and shared logic.
- `src/styles/`: Modular CSS files.
- `src/assets/`: Static assets like icons (SVG) and images.
- `src/router/`: Vue Router configuration.

### Type Safety

- Always use TypeScript for all new code.
- Define shared interfaces and types in `src/types.ts`.
- Avoid using `any`; define proper types for API responses and component state.

### API Interaction

- Use the native `fetch` API for network requests.
- Centralize API logic within Pinia actions to keep components clean.
- Always include error handling and loading states for asynchronous operations.

When creating new features, always check `src/styles/` first to see if a utility or component class already exists before creating new ones.

## 10. Intentional Design Decisions

This section documents choices that look like mistakes but are deliberate. Do not "fix" them without understanding the rationale.

### Password entropy pool size (`src/utils/passwordStrength.ts`)

The special-character pool is hardcoded to **16**, not the full ~32 printable ASCII symbols. This is intentional: it approximates the subset of symbols users realistically type on typical Latin keyboard layouts, rather than the theoretical maximum. Using 32 would over-estimate entropy for real-world passwords. The slight under-estimation is safe — it makes the strength requirement marginally stricter than the entropy formula strictly demands. The inline comments in `passwordStrength.ts` explain this in detail.
