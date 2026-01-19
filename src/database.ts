import Database from "better-sqlite3";
import { join } from "path";
import { runtimeData } from "./config";
import type { Server } from "./types/server";
import type { Site } from "./types/site";

let dbInstance: Database.Database | null = null;

/**
 * Initialize and get the database connection.
 * The database file is stored in runtimeData.dataDir.
 */
export function getDb(): Database.Database {
  if (dbInstance) return dbInstance;

  const dbPath = join(runtimeData.dataDir, "jman.db");
  dbInstance = new Database(dbPath);

  // Enable foreign keys
  dbInstance.pragma("foreign_keys = ON");

  // Initialize tables
  dbInstance.exec(`
    CREATE TABLE IF NOT EXISTS servers (
      id INTEGER PRIMARY KEY,
      name TEXT NOT NULL,
      ip_address TEXT,
      ssh_port INTEGER,
      data TEXT NOT NULL
    );

    CREATE TABLE IF NOT EXISTS sites (
      id INTEGER PRIMARY KEY,
      server_id INTEGER NOT NULL,
      domain TEXT NOT NULL,
      site_user TEXT,
      public_folder TEXT,
      data TEXT NOT NULL,
      FOREIGN KEY (server_id) REFERENCES servers (id) ON DELETE CASCADE
    );
  `);

  return dbInstance;
}

/**
 * Persist servers to the database.
 */
export function saveServers(servers: Server[]) {
  const db = getDb();
  const insert = db.prepare(`
    INSERT OR REPLACE INTO servers (id, name, ip_address, ssh_port, data)
    VALUES (?, ?, ?, ?, ?)
  `);

  const transaction = db.transaction((servers: Server[]) => {
    for (const server of servers) {
      insert.run(
        server.id,
        server.name,
        server.ip_address,
        server.ssh_port,
        JSON.stringify(server),
      );
    }
  });

  transaction(servers);
}

/**
 * Persist sites to the database.
 */
export function saveSites(sites: Site[]) {
  const db = getDb();
  const insert = db.prepare(`
    INSERT OR REPLACE INTO sites (id, server_id, domain, site_user, public_folder, data)
    VALUES (?, ?, ?, ?, ?, ?)
  `);

  const transaction = db.transaction((sites: Site[]) => {
    for (const site of sites) {
      insert.run(
        site.id,
        site.server_id,
        site.domain,
        site.site_user,
        site.public_folder,
        JSON.stringify(site),
      );
    }
  });

  transaction(sites);
}

/**
 * Retrieve all servers from the database.
 */
export function getServersFromDb(): Server[] {
  const db = getDb();
  const rows = db.prepare("SELECT data FROM servers").all() as {
    data: string;
  }[];
  return rows.map((row) => JSON.parse(row.data));
}

/**
 * Retrieve all sites from the database.
 */
export function getSitesFromDb(): Site[] {
  const db = getDb();
  const rows = db.prepare("SELECT data FROM sites").all() as {
    data: string;
  }[];
  return rows.map((row) => JSON.parse(row.data));
}

/**
 * Retrieve a specific server by ID.
 */
export function getServerById(id: number): Server | undefined {
  const db = getDb();
  const row = db.prepare("SELECT data FROM servers WHERE id = ?").get(id) as
    | { data: string }
    | undefined;
  return row ? JSON.parse(row.data) : undefined;
}

/**
 * Retrieve a specific site by ID.
 */
export function getSiteById(id: number): Site | undefined {
  const db = getDb();
  const row = db.prepare("SELECT data FROM sites WHERE id = ?").get(id) as
    | { data: string }
    | undefined;
  return row ? JSON.parse(row.data) : undefined;
}

/**
 * Clear all data from the database.
 */
export function clearDatabase() {
  const db = getDb();
  db.exec("DELETE FROM sites; DELETE FROM servers;");
}
