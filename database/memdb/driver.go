package memdb

import (
	"fmt"

	"github.com/btcsuite/btclog"
	"github.com/monetas/bmd/database"
)

var log = btclog.Disabled

func init() {
	driver := database.DriverDB{DbType: "memdb", CreateDB: CreateDB, OpenDB: OpenDB}
	database.AddDBDriver(driver)
}

// parseArgs parses the arguments from the database package Open/Create methods.
func parseArgs(funcName string, args ...interface{}) error {
	if len(args) != 0 {
		return fmt.Errorf("memdb.%s does not accept any arguments",
			funcName)
	}

	return nil
}

// OpenDB opens an existing database for use.
func OpenDB(args ...interface{}) (database.Db, error) {
	if err := parseArgs("OpenDB", args...); err != nil {
		return nil, err
	}

	// A memory database is not persistent, so let CreateDB handle it.
	return CreateDB()
}

// CreateDB creates, initializes, and opens a database for use.
func CreateDB(args ...interface{}) (database.Db, error) {
	if err := parseArgs("CreateDB", args...); err != nil {
		return nil, err
	}

	log = database.GetLog()
	return newMemDb(), nil
}
