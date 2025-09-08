import z from "zod";

const domainRedirectSchema = z.object({
  enabled: z.boolean(),
});

const additionalDomainSchema = z.object({
  domain: z.string(),
  redirect: domainRedirectSchema,
  created_at: z.string(),
});

const pageCacheSchema = z.object({
  enabled: z.boolean(),
});

const httpsSchema = z.object({
  enabled: z.boolean(),
  certificate_expires: z.string().default(""),
  certificate_renews: z.string().default(""),
});

const nginxSchema = z.object({
  uploads_directory_protected: z.boolean(),
  xmlrpc_protected: z.boolean(),
  subdirectory_rewrite_in_place: z.boolean(),
});

const databaseSchema = z.object({
  id: z.number(),
  user_id: z.number().nullable().optional(),
  table_prefix: z.string().nullable().optional(),
});

const storageProviderSchema = z.object({
  id: z.number().nullable().optional(),
  region: z.string().nullable().optional(),
  bucket: z.string().nullable().optional(),
});

const backupsSchema = z.object({
  files: z.boolean(),
  database: z.boolean(),
  paths_to_exclude: z.string().nullable().optional(),
  is_backups_retention_period_enabled: z.boolean().nullable().optional(),
  retention_period: z.number().nullable().optional(),
  next_run_time: z.string().nullable().optional(),
  storage_provider: storageProviderSchema.nullable().optional(),
});

const gitSchema = z.object({
  enabled: z.boolean(),
  repo: z.string().nullable().optional(),
  branch: z.string().nullable().optional(),
  deploy_script: z.string().nullable().optional(),
  push_enabled: z.boolean().nullable().optional(),
  deployment_url: z.string().nullable().optional(),
});

const basicAuthSchema = z.object({
  enabled: z.boolean(),
  username: z.string().nullable().optional(),
});

export const siteSchema = z.object({
  id: z.number(),
  server_id: z.number(),
  domain: z.string().default(""),
  additional_domains: z.array(additionalDomainSchema),
  site_user: z.string().default(""),
  user_auth: z.string().default(""),
  php_version: z.string().default(""),
  public_folder: z.string().default(""),
  is_wordpress: z.boolean(),
  page_cache: pageCacheSchema,
  https: httpsSchema,
  nginx: nginxSchema,
  database: databaseSchema,
  backups: backupsSchema,
  wp_core_update: z.boolean(),
  wp_theme_updates: z.number().default(0),
  wp_plugin_updates: z.number().default(0),
  git: gitSchema,
  basic_auth: basicAuthSchema,
  created_at: z.string(),
  status: z.string(),
});

export type Site = z.infer<typeof siteSchema>;
