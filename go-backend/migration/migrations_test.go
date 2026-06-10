package migration

import (
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", dbName)), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	require.NoError(t, err)

	return db
}

func TestMigrate002NodePortRangesSkipsCurrentSchemaWithoutLegacyColumns(t *testing.T) {
	db := openTestDB(t)
	require.NoError(t, db.Exec(`
		CREATE TABLE node (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			port_ranges TEXT DEFAULT ''
		)
	`).Error)

	require.NoError(t, migrate002NodePortRanges(db))
	require.True(t, columnExists(db, "node", "port_ranges"))
}

func TestMigrate002NodePortRangesMigratesLegacyPortColumns(t *testing.T) {
	db := openTestDB(t)
	require.NoError(t, db.Exec(`
		CREATE TABLE node (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			port_sta INTEGER,
			port_end INTEGER
		)
	`).Error)
	require.NoError(t, db.Exec("INSERT INTO node (port_sta, port_end) VALUES (1000, 1000), (2000, 3000)").Error)

	require.NoError(t, migrate002NodePortRanges(db))

	var ranges []string
	require.NoError(t, db.Raw("SELECT port_ranges FROM node ORDER BY id").Scan(&ranges).Error)
	require.Equal(t, []string{"1000", "2000-3000"}, ranges)
}

func TestMigrate003ForwardPauseReasonAddsColumn(t *testing.T) {
	db := openTestDB(t)
	require.NoError(t, db.Exec(`
		CREATE TABLE forward (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			status INTEGER
		)
	`).Error)

	require.NoError(t, migrate003ForwardPauseReason(db))
	require.True(t, columnExists(db, "forward", "pause_reason"))

	var value int
	require.NoError(t, db.Exec("INSERT INTO forward (status) VALUES (0)").Error)
	require.NoError(t, db.Raw("SELECT pause_reason FROM forward LIMIT 1").Scan(&value).Error)
	require.Equal(t, 0, value)

	require.NoError(t, migrate003ForwardPauseReason(db))
}
