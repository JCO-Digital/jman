import z from "zod";

export const diskSpaceSchema = z.object({
  total: z.number().default(0),
  available: z.number().default(0),
  used: z.number().default(0),
  updated_at: z.string().default(""),
});

export type DiskSpace = z.infer<typeof diskSpaceSchema>;

export const databaseSchema = z.object({
  server: z.string().default(""),
  host: z.string().default(""),
  port: z.coerce.number().default(0),
});

export type Database = z.infer<typeof databaseSchema>;

export const serverSchema = z.object({
  id: z.number().default(0),
  name: z.string().default(""),
  provider_name: z.string().default(""),
  ubuntu_version: z.string().default(""),
  ip_address: z.string().default(""),
  ssh_port: z.number().default(0),
  timezone: z.string().default(""),
  region: z.string().default(""),
  size: z.string().default(""),
  disk_space: diskSpaceSchema,
  database: databaseSchema,
  ssh_publickey: z.string().default(""),
  git_publickey: z.string().default(""),
  connection_status: z.string().default(""),
  reboot_required: z.boolean().default(false),
  upgrade_required: z.boolean().default(false),
  install_notes: z
    .string()
    .nullish()
    .transform((s) => s ?? ""),
  created_at: z.string().default(""),
  status: z.string().default(""),
});

export type Server = z.infer<typeof serverSchema>;
