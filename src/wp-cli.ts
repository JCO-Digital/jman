import { exec } from "child_process";
import { runtimeData } from "./config";

type RunWPReturn = {
  output: string;
  error: string;
};

export function runWP(
  ssh: string,
  path: string,
  command: string,
): Promise<RunWPReturn> {
  const options = {
    cwd: runtimeData.cacheDir,
  };

  return new Promise((resolve, reject) => {
    exec(
      `wp --ssh=${ssh} --path=${path} --skip-plugins --skip-themes ${command}`,
      options,
      (error, stdout, stderr) => {
        if (error) {
          console.error(`exec error: ${error}`);
          reject(error);
          return;
        }
        resolve({ output: stdout, error: stderr });
      },
    );
  });
}
