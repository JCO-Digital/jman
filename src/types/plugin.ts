import z from "zod";

export const pluginSchema = z.object({
  site_id: z.number(),
  name: z.string(),
  status: z.string().default(""),
  version: z.string().default(""),
  update: z.string().default(""),
  autoUpdate: z.boolean().default(false),
});

export type WpPlugin = z.infer<typeof pluginSchema>;

export const pluginSiteSchema = z.object({
  site_id: z.number(),
  version: z.string().default(""),
});

export const pluginDataSchema = z.object({
  name: z.string(),
  sites: z.array(pluginSiteSchema).default([]),
});

export type WpPluginData = z.infer<typeof pluginDataSchema>;
