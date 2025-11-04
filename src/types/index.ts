import { z } from "zod";

export const runtimeSchema = z.object({
  configDir: z.string(),
  cacheDir: z.string(),
  dataDir: z.string(),
  nodePath: z.string().default(""),
  scriptPath: z.string().default(""),
});

export type jRuntime = z.infer<typeof runtimeSchema>;

export const configSchema = z.object({
  urlMainwp: z.string().default("https://mainwp.jcore.fi/wp-json/mainwp/v2"),
  tokenSpinup: z.string().default(""),
  tokenMainwp: z.string().default(""),
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
