import { writeFile } from "fs";

/**
 * Downloads a specific release of jman by tag and saves it to the target path.
 *
 * @param {string} tag - The GitHub release tag.
 * @param {string} target - The destination file path.
 * @returns {Promise<void>}
 */
export async function downloadReleaseByTag(
  tag: string,
  target: string,
): Promise<void> {
  const url = `https://github.com/JCO-Digital/jman/releases/download/${tag}/jman`;

  const buffer = await getFile(url, (progress) => {
    const percentage = Math.round(progress * 100);
    process.stdout.write(`\rDownloading update: ${percentage}% `);
  });

  process.stdout.write("\n");

  return new Promise((resolve, reject) => {
    writeFile(target, buffer, (err) => {
      if (err) reject(err);
      resolve();
    });
  });
}

/**
 * Fetches a file from a given URL and returns it as a Buffer.
 * Supports an optional progress callback.
 *
 * @param {string} url - The URL of the file to fetch.
 * @param {function} onProgress - Optional callback receiving a float from 0 to 1.
 * @returns {Promise<Buffer>} Promise that resolves with the file content as a Buffer.
 */
export async function getFile(
  url: string,
  onProgress?: (progress: number) => void,
): Promise<Buffer> {
  const response = await fetch(url);

  if (!response.ok) {
    throw new Error(`Failed to fetch ${url}: ${response.statusText}`);
  }

  const contentLength = response.headers.get("content-length");
  const total = contentLength ? parseInt(contentLength, 10) : 0;

  if (!total || !response.body || !onProgress) {
    const buffer = await response.arrayBuffer();
    return Buffer.from(buffer);
  }

  const reader = response.body.getReader();
  let loaded = 0;
  const chunks: Uint8Array[] = [];

  while (true) {
    const { done, value } = await reader.read();

    if (done) break;

    chunks.push(value);
    loaded += value.length;
    onProgress(loaded / total);
  }

  const result = new Uint8Array(loaded);
  let offset = 0;
  for (const chunk of chunks) {
    result.set(chunk, offset);
    offset += chunk.length;
  }

  return Buffer.from(result);
}

/**
 * Saves a Buffer to a file.
 *
 * @param {string} filePath - Path where the file should be saved.
 * @param {Buffer} buffer - Content to save.
 * @returns {Promise<void>}
 */
export function saveFile(filePath: string, buffer: Buffer): Promise<void> {
  return new Promise((resolve, reject) => {
    writeFile(filePath, buffer, (err) => {
      if (err) reject(err);
      resolve();
    });
  });
}
