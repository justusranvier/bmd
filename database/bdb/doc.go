// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package bdb implements an instance of the database package backed by BoltDB.
// The structure of the database is:
// - objectsByHashes (bucket)
// -- Inventory hash (32 bytes) -> Object data
//
// - objectsByCounters (bucket)
// -- Getpubkey/Pubkey/Msg/Broadcast/Unknown (bucket)
// --- Counter value (uint64) -> Inventory hash (32 bytes)
//
// - counterPositions (bucket)
// -- Getpubkey/Pubkey/Msg/Broadcast/Unknown -> uint64
//
// - encryptedPubkeysByTag (bucket)
// -- Tag (32 bytes) -> Encrypted pubkey
//
// - publicIdentityByAddress (bucket)
// -- Address (string starting with BM-) (bucket)
// --- nonceTrials   -> uint64
// --- extraBytes    -> uint64
// --- signingKey    -> compressed public key (33 bytes)
// --- encryptionKey -> compressed public key (33 bytes)
// --- behavior      -> uint32
//
// - misc
// -- version -> uint8
package bdb
