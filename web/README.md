# JMan UI

JMan UI is a modern web interface built with Vue 3, TypeScript, and Vite for managing servers, sites, and their associated plugins.

## Features

- **Dashboard:** A central view to monitor all sites, showing server association and plugin counts.
- **Data Table:** Sortable and paginated data table for easy navigation of large amounts of sites.
- **Search:** Quickly find specific sites by domain or name.
- **Site Details:** Detailed view for individual sites (routing setup for `/site/:id`).
- **Data Caching:** Client-side caching using `localStorage` to ensure fast load times and minimal API requests.
- **Theming:** Built-in support for Light and Dark modes based on system preferences using CSS variables.

## Tech Stack

- [Vue 3](https://vuejs.org/) (using `<script setup>` and Composition API)
- [TypeScript](https://www.typescriptlang.org/)
- [Vite](https://vitejs.dev/)
- [Vue Router](https://router.vuejs.org/)

## Prerequisites

Make sure you have the following installed on your machine:

- [Node.js](https://nodejs.org/) (v18+ recommended)
- [pnpm](https://pnpm.io/) (used as the package manager)

## Getting Started

### 1. Clone the repository and install dependencies

```bash
# Install dependencies
pnpm install
```

### 2. Environment Setup

By default, the application expects the API to be available at `/api`. You can override this by creating a `.env` file in the root directory:

```env
VITE_API_BASE_URL=http://localhost:3000/api
```

### 3. Run the development server

```bash
# Start the Vite development server
pnpm run dev
```

The app will be available at `http://localhost:5173`.

### 4. Build for Production

```bash
# Type-check and build the app for production
pnpm run build

# Preview the production build locally
pnpm run preview
```

## Project Structure

```text
jman-ui/
├── public/             # Static assets
├── src/
│   ├── assets/         # Project assets (images, fonts, etc.)
│   ├── components/     # Reusable Vue components
│   ├── composables/    # Vue Composition API functions (e.g., useData.ts)
│   ├── router/         # Vue Router configuration
│   ├── views/          # Page-level components (HomeView, SiteDetailView)
│   ├── App.vue         # Root component
│   ├── main.ts         # Application entry point
│   ├── style.css       # Global styles and CSS variables
│   └── types.ts        # TypeScript interfaces and type definitions
├── package.json
├── pnpm-lock.yaml
├── tsconfig.json
└── vite.config.ts
```

## Data Management & API

State management is handled through a lightweight shared composable (`src/composables/useData.ts`). It fetches data from three primary endpoints:

- `/servers`
- `/sites`
- `/plugins`

The composable automatically caches the API responses in `localStorage` (`jman_servers`, `jman_sites`, `jman_plugins`). On subsequent loads, it serves the cached data instantly while allowing the user to manually refresh the data via the UI to sync with the backend.

## Theming

The application uses CSS variables defined in `src/style.css` to manage colors and theming. It automatically respects the user's system preferences for light or dark mode via the `@media (prefers-color-scheme: dark)` media query.

To modify the theme, you can adjust the CSS variables found in `:root` and the dark mode overrides.
