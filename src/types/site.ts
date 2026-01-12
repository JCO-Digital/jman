import z from "zod";

const enabledSchema = z.object({
  enabled: z.boolean(),
});

const additionalDomainSchema = z.object({
  domain: z.string(),
  redirect: enabledSchema,
  created_at: z.string(),
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
  id: z.number().nullish(),
  user_id: z.number().nullish(),
  table_prefix: z.string().nullish(),
});

const storageProviderSchema = z.object({
  id: z.number().nullish(),
  region: z.string().nullish(),
  bucket: z.string().nullish(),
});

const backupsSchema = z.object({
  files: z.boolean(),
  database: z.boolean(),
  paths_to_exclude: z.string().nullish(),
  is_backups_retention_period_enabled: z.boolean().nullish(),
  retention_period: z.number().nullish(),
  next_run_time: z.string().nullish(),
  storage_provider: storageProviderSchema.nullish(),
});

const basicAuthSchema = z.object({
  enabled: z.boolean(),
  username: z.string().nullish(),
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
  page_cache: enabledSchema,
  https: httpsSchema,
  nginx: nginxSchema,
  database: databaseSchema,
  backups: backupsSchema,
  wp_core_update: z.boolean(),
  wp_theme_updates: z.number().default(0),
  wp_plugin_updates: z.number().default(0),
  basic_auth: basicAuthSchema,
  created_at: z.string(),
  status: z.string(),
});

export type Site = z.infer<typeof siteSchema>;

export const cliSiteSchema = z.object({
  id: z.number(),
  name: z.string(),
  serverId: z.number(),
  serverName: z.string(),
  ssh: z.string(),
  path: z.string(),
});

export type CliSite = z.infer<typeof cliSiteSchema>;
