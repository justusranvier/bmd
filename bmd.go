// Originally derived from: btcsuite/btcd/btcd.go
// Copyright (c) 2013-2015 Conformal Systems LLC.

// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net"
	"os"
	"runtime"
)

var (
	shutdownChannel = make(chan struct{})
)

// bmdMain is the real main function for bmd. It is necessary to work around
// the fact that deferred functions do not run when os.Exit() is called. The
func bmdMain() error {
	listeners := make([]string, 1)
	listeners[0] = net.JoinHostPort("", "8445")

	db, err := setupDB("memdb", "")
	if err != nil {
		return err
	}

	// Create server and start it.
	server, err := NewServer(listeners, db)
	if err != nil {
		return err
	}
	server.Start()

	// Monitor for graceful server shutdown and signal the main goroutine
	// when done. This is done in a separate goroutine rather than waiting
	// directly so the main goroutine can be signaled for shutdown by either
	// a graceful shutdown or from the main interrupt handler. This is
	// necessary since the main goroutine must be kept running long enough
	// for the interrupt handler goroutine to finish.
	go func() {
		server.WaitForShutdown()
		shutdownChannel <- struct{}{}
	}()

	// Wait for shutdown signal from either a graceful server stop or from
	// the interrupt handler.
	<-shutdownChannel
	return nil
}

func main() {
	// Use all processor cores.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Work around defer not working after os.Exit()
	if err := bmdMain(); err != nil {
		fmt.Printf("err %v\n", err)
		os.Exit(1)
	}
}
