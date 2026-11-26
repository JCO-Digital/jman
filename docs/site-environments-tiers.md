# Site environments and tiers

Two ways to organize sites beyond their environment classification
(`production`, `staging`, `development`, `archived`).

## Environment links

A staging, development or archived site can be linked to the production
site it's a copy of (`site_production_link` in inventory.db). A site has at
most one production site; a production site can have any number of linked
sites. Links don't chain:

- the linked site must be staging, development or archived,
- the production site must be production and not linked itself,
- a site that others are linked to can't be linked.

`GET /api/sites` reports a linked site's `production_site_id`. Changing a
site's environment later doesn't remove its links; the web UI marks a link
whose environments no longer fit (e.g. a staging site made production) so
it can be fixed by hand. Deleting a site from the inventory removes the
links to and from it.

### In the web UI

The site detail's **Environments** card shows the production site and its
linked sites, from either side, with each one's server, WordPress version
and last update. Below it, **Version differences** compares WordPress core,
PHP and every plugin across them, production first: a newer version than
production is green, an older one amber, a missing plugin "—", and an
inactive copy is marked. "Only differences" (on by default) hides the rows
that match everywhere.

Links are made by hand from the card, which also suggests them for
unlinked sites from the domains:

| Linked site | Production site |
| --- | --- |
| `staging.example.com`, `www.dev.example.com` | `example.com` |
| `staging-example.com`, `example-dev.com` | `example.com` |
| `sitename.dev.bojaco.com`, `sitename.staging.bojaco.com` | every production site named `sitename`: `sitename.fi`, `www.sitename.com` |

Names match ignoring case and hyphens (`site-name.dev.bojaco.com` →
`sitename.fi`). When several production sites match, the card offers each.

The sites list shows "staging of example.com" under a linked site's domain.

## Tiers

Production sites can have a service tier (`site_tier` in inventory.db).
Other sites report no tier, even one stored from when they were production;
the site detail of a linked site shows its production site's tier instead.

The tiers are configured in jman-api's `config.toml`:

```toml
[[siteTiers]]
id = 10
name = "Basic"

[[siteTiers]]
id = 20
name = "Standard"
```

Without `siteTiers`, the defaults are Basic (10), Standard (20), Premium (30),
Enterprise (40) and Internal (90) (`models.DefaultSiteTiers`).

- **The id is the tier.** Sites store it, and it ranks the tiers: a higher
  id is a higher tier. Code that behaves differently by tier compares ids
  against the constants in `internal/models/site_tier.go`
  (`site.Tier >= models.TierPremium`), never names.
- **Names are display only** and can be renamed freely.
- **Leave gaps** between ids, so a new tier can go between two others
  (e.g. 25 between Standard and Premium) without renumbering.
- **Don't reuse an id** for a different tier. A removed tier's id stays on
  its sites and shows as "Tier <id>" until they're given another.

jman-api refuses to start if a tier has a non-positive or duplicate id or
an empty name. `GET /api/site-tiers` serves the list to the UI.

Tiers are a label and a filter for now (the sites list's Tier column and
filter, the site detail badge). Tier-specific behavior is to come.
