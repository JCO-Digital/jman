package db

import (
	"database/sql"

	"github.com/JCO-Digital/jman/internal/verb"
)

// inventoryProviderDataMigrationName is the api.db schema_migrations key
// recording that inventory provider_data was re-keyed by provider.
const inventoryProviderDataMigrationName = "inventory_provider_data_by_provider"

// KeyInventoryProviderData moves each inventory item's provider_data from
// one flat snapshot to a map keyed by provider name, so that an item linked
// to several providers keeps one snapshot from each. Until now SpinupWP was
// the only provider, so every existing snapshot is SpinupWP's, including
// those of items whose links have since gone.
func KeyInventoryProviderData(dbConn *sql.DB) error {
	return runOnce(dbConn, inventoryProviderDataMigrationName, func(tx *sql.Tx) error {
		res, err := tx.Exec(`UPDATE inventory_items
			SET provider_data = json_object('spinupwp', json(provider_data))
			WHERE provider_data NOT IN ('', '{}', 'null')`)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		verb.LogPrintf(verb.Normal, "Keyed the provider data of %d inventory item(s) by provider", n)
		return nil
	})
}
