// Originally derived from: btcsuite/btcd/database/memdb/driver.go
// Copyright (c) 2013-2015 Conformal Systems LLC.

// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package memdb

import (
	"fmt"

	"github.com/btcsuite/btclog"
	"github.com/monetas/bmd/database"
)

var log = btclog.Disabled

func init() {
	driver := database.DriverDB{DbType: "memdb", OpenDB: OpenDB}
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

// OpenDB opens a database, initializing it if necessary.
func OpenDB(args ...interface{}) (database.Db, error) {
	if err := parseArgs("OpenDB", args...); err != nil {
		return nil, err
	}

	log = database.GetLog()

	return newMemDb(), nil
}
