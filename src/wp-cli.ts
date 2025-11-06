import { exec } from "child_process";
import { runtimeData } from "./config";

export type RunWPArgs = {
  ssh: string;
  path: string;
};

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
          reject(error);
          return;
        }
        resolve({ output: stdout, error: stderr });
      },
    );
  });
}

export async function addUser(
  ssh: string,
  path: string,
  username: string,
  email: string,
  role: string,
): Promise<string> {
  const ret = await runWP(
    ssh,
    path,
    `user create ${username} ${email} --role=${role}`,
  );
  const password = ret.output.match(/Password: (.+)/);
  if (password?.length) {
    return password[1];
  }
  return "";
}

export async function resetUserPassword(
  ssh: string,
  path: string,
  username: string,
): Promise<string> {
  const ret = await runWP(
    ssh,
    path,
    `user reset-password ${username} --porcelain`,
  );
  return ret.output;
}

export async function addPlugin(
  ssh: string,
  path: string,
  plugin: string,
): Promise<boolean> {
  const ret = await runWP(ssh, path, `plugin install ${plugin} --activate`);
  return ret.output.includes("Success:");
}

export async function isActiveMainwp(
  ssh: string,
  path: string,
): Promise<boolean> {
  try {
    await runWP(ssh, path, `plugin is-active mainwp-child`);
    return true;
  } catch (error) {
    if (error.toString().includes("not found")) {
      console.error(`Can't connect to ${ssh}`);
    }
  }
  return false;
}

export async function setDisallowFileMods(
  ssh: string,
  path: string,
  value = true,
) {
  try {
    const valueString = value ? "true" : "false";
    await runWP(
      ssh,
      path,
      `config set --raw DISALLOW_FILE_MODS ${valueString}`,
    );
  } catch (error) {
    if (error.toString().includes("not found")) {
      console.error(`Can't connect to ${ssh}`);
    }
  }
}
