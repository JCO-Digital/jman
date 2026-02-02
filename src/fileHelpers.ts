import { writeFile } from "fs";

export function downloadReleaseByTag(
  tag: string,
  target: string,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const url = `https://github.com/JCO-Digital/jman/releases/download/${tag}/jman`;
    console.warn(`Downloading release ${tag} to ${target}`);

    getFile(url).then((buffer) => {
      writeFile(target, buffer, (err) => {
        if (err) reject(err);
        resolve();
      });
    });
  });
}

/**
 * Fetches a file from a given URL and returns it as a Buffer.
 *
 * @param {string} url - The URL of the file to fetch.
 * @returns {Promise<Buffer>} Promise that resolves with the file content as a Buffer.
 */
export async function getFile(url: string): Promise<Buffer> {
  const response = await fetch(url);
  const buffer = await response.arrayBuffer();
  return Buffer.from(buffer);
}

export function saveFile(filePath: string, buffer: Buffer): Promise<void> {
  return new Promise((resolve, reject) => {
    writeFile(filePath, buffer, (err) => {
      if (err) reject(err);
      resolve();
    });
  });
}
