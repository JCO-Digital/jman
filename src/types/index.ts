import { z } from "zod";

export const runtimeSchema = z.object({
  configDir: z.string(),
  cacheDir: z.string(),
  dataDir: z.string(),
  version: z.string().default(""),
  nodePath: z.string().default(""),
  scriptPath: z.string().default(""),
  execPath: z.string().default(""),
});

export type jRuntime = z.infer<typeof runtimeSchema>;

export const configSchema = z.object({
  urlMainwp: z.string().default(""),
  tokenSpinup: z.string().default(""),
  tokenMainwp: z.string().default(""),
  slackToken: z.string().default(""),
  slackChannel: z.string().default("#testing"),
  cvssThreshold: z.number().min(0).max(10).default(7),
  vulnThreshold: z.number().default(7),
  ignoreSites: z.array(z.string()).default([]),
});

export type jConfig = z.infer<typeof configSchema>;

export const cmdSchema = z.object({
  cmd: z.string().default(""),
  target: z.string().default(""),
  args: z.array(z.string()).default([]),
});

export type jCmd = z.infer<typeof cmdSchema>;

export type SpinupReply = {
  data: Array<object>;
  pagination?: {
    next: string | null;
    previous: string | null;
    per_page: number;
    count: number;
  };
};
