// Originally derived from: btcsuite/btcd/btcd.go
// Copyright (c) 2013-2015 Conformal Systems LLC.

// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"

	"github.com/monetas/bmd/database"
	_ "github.com/monetas/bmd/database/memdb"
	"github.com/monetas/bmd/peer"
)

const (
	// objectDbNamePrefix is the prefix for the object database name. The
	// database type is appended to this value to form the full object database
	// name.
	objectDbNamePrefix = "objects"
)

var (
	cfg             *config
	shutdownChannel = make(chan struct{})
)

// bmdMain is the real main function for bmd. It is necessary to work around
// the fact that deferred functions do not run when os.Exit() is called. The
func bmdMain() error {

	// Load configuration.
	tcfg, _, err := loadConfig(false)
	if err != nil {
		return err
	}
	cfg = tcfg
	defer backendLog.Flush()

	// Ensure that the correct dialer is used.
	peer.SetDialer(bmdDial)

	// Show version at startup.
	bmdLog.Infof("Version %s", version())

	// Enable http profiling server if requested.
	if cfg.Profile != "" {
		go func() {
			listenAddr := net.JoinHostPort("", cfg.Profile)
			bmdLog.Infof("Profile server listening on %s", listenAddr)
			profileRedirect := http.RedirectHandler("/debug/pprof",
				http.StatusSeeOther)
			http.Handle("/", profileRedirect)
			bmdLog.Errorf("%v", http.ListenAndServe(listenAddr, nil))
		}()
	}

	// Write cpu profile if requested.
	if cfg.CPUProfile != "" {
		f, err := os.Create(cfg.CPUProfile)
		if err != nil {
			bmdLog.Errorf("Unable to create cpu profile: %v", err)
			return err
		}
		pprof.StartCPUProfile(f)
		defer f.Close()
		defer pprof.StopCPUProfile()
	}

	// Load object database.
	db, err := setupDB(cfg.DbType, objectDbPath(cfg.DbType))
	if err != nil {
		dbLog.Errorf("Failed to initialize database: %v", err)
		return err
	}
	defer db.Close()

	// Ensure the database is sync'd and closed on Ctrl+C.
	addInterruptHandler(func() {
		bmdLog.Infof("Gracefully shutting down the database...")
		db.RollbackClose()
	})

	// Create server and start it.
	server, err := newDefaultServer(cfg.Listeners, db)
	if err != nil {
		serverLog.Errorf("Failed to start server on %v: %v", cfg.Listeners,
			err)
		return err
	}
	server.Start()

	addInterruptHandler(func() {
		bmdLog.Infof("Gracefully shutting down the server...")
		server.Stop()
		server.WaitForShutdown()
	})

	// Monitor for graceful server shutdown and signal the main goroutine
	// when done. This is done in a separate goroutine rather than waiting
	// directly so the main goroutine can be signaled for shutdown by either
	// a graceful shutdown or from the main interrupt handler. This is
	// necessary since the main goroutine must be kept running long enough
	// for the interrupt handler goroutine to finish.
	go func() {
		server.WaitForShutdown()
		serverLog.Info("Server shutdown complete")
		shutdownChannel <- struct{}{}
	}()

	// Wait for shutdown signal from either a graceful server stop or from
	// the interrupt handler.
	<-shutdownChannel
	bmdLog.Info("Shutdown complete")
	return nil
}

func main() {
	// Use all processor cores.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Work around defer not working after os.Exit()
	if err := bmdMain(); err != nil {
		os.Exit(1)
	}
}

// objectDbPath returns the path to the object database given a database type.
func objectDbPath(dbType string) string {
	// The database name is based on the database type.
	dbName := objectDbNamePrefix + "_" + dbType
	if dbType == "sqlite" {
		dbName = dbName + ".db"
	}
	dbPath := filepath.Join(cfg.DataDir, dbName)
	return dbPath
}

// warnMultipeDBs shows a warning if multiple database types are detected.
// This is not a situation most users want.  It is handy for development however
// to support multiple side-by-side databases.
func warnMultipeDBs(dbType string) {
	// This is intentionally not using the known db types which depend
	// on the database types compiled into the binary since we want to
	// detect legacy db types as well.
	dbTypes := []string{"leveldb", "sqlite"}
	duplicateDbPaths := make([]string, 0, len(dbTypes)-1)
	for _, dbt := range dbTypes {
		if dbt == dbType {
			continue
		}

		// Store db path as a duplicate db if it exists.
		/*dbPath := blockDbPath(dbType)
		if fileExists(dbPath) {
			duplicateDbPaths = append(duplicateDbPaths, dbPath)
		}*/
	}

	// Warn if there are extra databases.
	if len(duplicateDbPaths) > 0 {
		// TODO
	}
}

// setupDB loads (or creates when needed) the object database taking into
// account the selected database backend. It also contains additional logic
// such warning the user if there are multiple databases which consume space on
// the file system.
func setupDB(dbType, dbPath string) (database.Db, error) {
	// The memdb backend does not have a file path associated with it, so
	// handle it uniquely.  We also don't want to worry about the multiple
	// database type warnings when running with the memory database.
	if dbType == "memdb" {
		db, err := database.CreateDB(dbType)
		if err != nil {
			return nil, err
		}
		return db, nil
	}

	warnMultipeDBs(dbType)

	// The database name is based on the database type.
	//dbPath := blockDbPath(dbType)

	db, err := database.OpenDB(dbType, dbPath)
	if err != nil {
		// Return the error if it's not because the database
		// doesn't exist.
		if err != database.ErrDbDoesNotExist {
			return nil, err
		}

		// Create the db if it does not exist.
		/*err = os.MkdirAll(cfg.DataDir, 0700)
		if err != nil {
			return nil, err
		}*/
		db, err = database.CreateDB(dbType, dbPath)
		if err != nil {
			return nil, err
		}
	}

	return db, nil
}
