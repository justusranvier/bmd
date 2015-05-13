// Originally derived from: btcsuite/btcd/database/doc.go
// Copyright (c) 2013-2015 Conformal Systems LLC.

// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package database provides a database interface for handling Bitmessage objects.

Basic Design

The basic design of this package is to store objects to be propagated separate
from the objects that need to persist (pubkeys). The objects to be propagated
are periodically deleted as they expire. It uses counters instead of timestamps
to keep track of the order that objects were received in. High level operations
such as FetchIdentityByAddress are also provided for convenience.

Usage

At the highest level, the use of this packages just requires that you import it,
setup a database, insert some data into it, and optionally, query the data back.
*/
package database
