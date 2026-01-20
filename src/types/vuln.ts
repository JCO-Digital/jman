import { z } from "zod";
import { pluginSiteSchema } from "./plugin";

export const cweSchema = z.object({
  cwe: z.string(),
  name: z.string(),
  description: z.string(),
});

export const cvssSchema = z.object({
  version: z.string(),
  vector: z.string(),
  av: z.string(),
  ac: z.string(),
  pr: z.string(),
  ui: z.string(),
  s: z.string(),
  c: z.string(),
  i: z.string(),
  a: z.string(),
  score: z.string(),
  severity: z.string(),
  exploitable: z.string(),
  impact: z.string().nullable(),
});

export const impactSchema = z.object({
  cvss: cvssSchema.optional(),
  cwe: z.array(cweSchema).optional(),
});

export const sourceSchema = z.object({
  id: z.string(),
  name: z.string(),
  link: z.string(),
  description: z.string().nullable(),
  date: z.string().nullable(),
});

export const operatorSchema = z.object({
  min_version: z.string().nullable(),
  min_operator: z.string().nullable(),
  max_version: z.string().nullable(),
  max_operator: z.string().nullable(),
  unfixed: z.string(),
  closed: z.string(),
});

export const vulnerabilitySchema = z.object({
  uuid: z.string(),
  name: z.string(),
  description: z.string().nullable(),
  operator: operatorSchema,
  source: z.array(sourceSchema),
  impact: z.union([impactSchema, z.array(z.never())]),
});

export const vulnDataSchema = z.object({
  name: z.string().nullable(),
  plugin: z.string(),
  link: z.string().nullable(),
  latest: z.string().nullable(),
  vulnerability: z.array(vulnerabilitySchema).nullable(),
});

export const vulnResponseSchema = z.object({
  error: z.number().default(1),
  message: z.string().default("").nullable(),
  data: vulnDataSchema.optional(),
  updated: z.coerce.number().default(0),
});

export type Cwe = z.infer<typeof cweSchema>;
export type Cvss = z.infer<typeof cvssSchema>;
export type Impact = z.infer<typeof impactSchema>;
export type Source = z.infer<typeof sourceSchema>;
export type Operator = z.infer<typeof operatorSchema>;
export type Vulnerability = z.infer<typeof vulnerabilitySchema>;
export type VulnData = z.infer<typeof vulnDataSchema>;
export type VulnResponse = z.infer<typeof vulnResponseSchema>;

export const vulnReportSchema = z.object({
  plugin: z.string(),
  vulnerability: vulnerabilitySchema,
  sites: z.array(pluginSiteSchema),
});

export type VulnReport = z.infer<typeof vulnReportSchema>;
