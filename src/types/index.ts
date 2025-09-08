import { z } from "zod";

export const runtimeSchema = z.object({
  configDir: z.string(),
  cacheDir: z.string(),
  dataDir: z.string(),
});

export type jRuntime = z.infer<typeof runtimeSchema>;

export const configSchema = z.object({
  token: z.string().default(""),
});

export type jConfig = z.infer<typeof configSchema>;

export type SpinupReply = {
  data: Array<object>;
  pagination?: {
    next: string | null;
    previous: string | null;
    per_page: number;
    count: number;
  };
};
