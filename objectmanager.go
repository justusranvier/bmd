// Originally derived from: btcsuite/btcd/blockmanager.go
// Copyright (c) 2013-2015 the btcsuite developers.

// Copyright (c) 2013-2014 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// TODO all peers added to and from the peer handler are added to and from the object manager too!
// TODO process data messages.
//    * unrequested data means to disconnect. 
//    * data that is received is no longer requested. 
//    * and it goes in the database. 
// TODO process inv messages. 
//    * find all unknown objects.
//    * request each of them from some peer. 
//    * re-assign the ones that aren't received. 

package main

import (
	"container/list"
	"sync"
	"sync/atomic"

	"github.com/monetas/bmd/database"
	"github.com/monetas/bmd/bmpeer"
	"github.com/monetas/bmutil/wire"
)

const (
	chanBufferSize = 50
)

// newPeerMsg signifies a newly connected peer to the block handler.
type newPeerMsg struct {
	peer *peer
}

// objectMsg packages a bitmessage object message and the peer it came from together
// so the block handler has access to that information.
type objectMsg struct {
	object wire.Message
	peer  *peer
}

// invMsg packages a bitmessage inv message and the peer it came from together
// so the block handler has access to that information.
type invMsg struct {
	inv  *wire.MsgInv
	peer *peer
}

// donePeerMsg signifies a newly disconnected peer to the block handler.
type donePeerMsg struct {
	peer *peer
}

// pauseMsg is a message type to be sent across the message channel for
// pausing the block manager.  This effectively provides the caller with
// exclusive access over the manager until a receive is performed on the
// unpause channel.
type pauseMsg struct {
	unpause <-chan struct{}
}

// What objects do we know about but don't have? 
// Who has them?
// Which have been requested and of whom? 
// objectManager provides a concurrency safe object manager for handling all
// incoming objects.
type objectManager struct {
	server            *server
	started           int32
	shutdown          int32
	requestedObjects  map[wire.ShaHash]*peer
	msgChan           chan interface{}
	wg                sync.WaitGroup
	quit              chan struct{}
}

// handleNewPeerMsg deals with new peers that have signalled they may
// be considered as a sync peer (they have already successfully negotiated).  It
// also starts syncing if needed.  It is invoked from the syncHandler goroutine.
func (b *objectManager) handleNewPeerMsg(peers *list.List, p *peer) {
	// Ignore if in the process of shutting down.
	if atomic.LoadInt32(&b.shutdown) != 0 {
		return
	}

	// Add the peer as a candidate to sync from.
	peers.PushBack(p)

	// Start syncing by choosing the best candidate if needed.
	//b.startSync(peers) // TODO figure out what function should be here.
}

// handleDonePeerMsg deals with peers that have signalled they are done.  It
// removes the peer as a candidate for syncing and in the case where it was
// the current sync peer, attempts to select a new best peer to sync from.  It
// is invoked from the syncHandler goroutine.
func (om *objectManager) handleDonePeerMsg(peers *list.List, p *peer) {
	// Remove the peer from the list of candidate peers.
	/*for e := peers.Front(); e != nil; e = e.Next() {
		if e.Value == p {
			peers.Remove(e)
			break
		}
	}

	// Remove requested transactions from the global map so that they will
	// be fetched from elsewhere next time we get an inv.
	for k := range p.requestedObjects {
		delete(om.requestedObjects, k)
	}

	// Remove requested blocks from the global map so that they will be
	// fetched from elsewhere next time we get an inv.
	// TODO(oga) we could possibly here check which peers have these blocks
	// and request them now to speed things up a little.
	for k := range p.requestedObjects {
		delete(om.requestedObjects, k)
	}*/
}

// handleObjectMsg handles transaction messages from all peers.
func (om *objectManager) handleObjectMsg(obj wire.Message) {	
	delete(om.requestedObjects, *wire.MessageHash(obj))
	
	om.server.db.InsertObject(wire.EncodeMessage(obj))
}

// haveInventory returns whether or not the inventory represented by the passed
// inventory vector is known.  This includes checking all of the various places
// inventory can be when it is in different states such as blocks that are part
// of the main chain, on a side chain, in the orphan pool, and transactions that
// are in the memory pool (either the main pool or orphan pool).
func (om *objectManager) haveInventory(invVect *wire.InvVect) (bool, error) {
	return om.server.db.ExistsObject(&invVect.Hash)
}

// handleInvMsg handles inv messages from all peers.
// We examine the inventory advertised by the remote peer and act accordingly.
func (om *objectManager) handleInvMsg(imsg *invMsg) {
	requestQueue := make([]*wire.InvVect, len(imsg.inv.InvList))

	// Request the advertised inventory if we don't already have it.  
	var i int = 0
	for _, iv := range imsg.inv.InvList {
		// Request the inventory if we don't already have it.
		haveInv, err := om.haveInventory(iv)
		if err != nil {
			continue
		}
		
		if !haveInv {
			// Add it to the request queue.
			requestQueue[i] = iv
			i++
			om.requestedObjects[iv.Hash] = imsg.peer
		}
	}
	
	if i == 0 {
		return
	}

	// TODO This hack sends the getData message to ALL peers, which is completely dumb.
	om.server.state.forAllPeers(func (p *bmpeer.Peer) {
		p.Logic().PushGetDataMsg(requestQueue[:i])
	})
}

// objectHandler is the main handler for the object manager.  It must be run
// as a goroutine.  It processes inv messages in a separate goroutine
// from the peer handlers. 
func (b *objectManager) objectHandler() {
	candidatePeers := list.New()
out:
	for {
		select {
		case m := <-b.msgChan:
			switch msg := m.(type) {
			case *newPeerMsg:
				b.handleNewPeerMsg(candidatePeers, msg.peer)

			case *objectMsg:
				b.handleObjectMsg((*objectMsg(msg)).object)
				// TODO should this line be there? 
				//msg.peer.objectProcessed <- struct{}{}

			case *invMsg:
				b.handleInvMsg(msg)

			case *donePeerMsg:
				b.handleDonePeerMsg(candidatePeers, msg.peer)

			case pauseMsg:
				// Wait until the sender unpauses the manager.
				<-msg.unpause

			default:
			}

		case <-b.quit:
			break out
		}
	}

	b.wg.Done()
}

// NewPeer informs the block manager of a newly active peer.
func (om *objectManager) NewPeer(p *peer) {
	// Ignore if we are shutting down.
	if atomic.LoadInt32(&om.shutdown) != 0 {
		return
	}

	om.msgChan <- &newPeerMsg{peer: p}
}

// QueueObject adds the passed block message and peer to the block handling queue.
func (om *objectManager) QueueObject(object wire.Message, p *peer) {
	// Don't accept more blocks if we're shutting down.
	if atomic.LoadInt32(&om.shutdown) != 0 {
		return
	}

	om.msgChan <- &objectMsg{object: object, peer: p}
}

// QueueInv adds the passed inv message and peer to the block handling queue.
func (om *objectManager) QueueInv(inv *wire.MsgInv, p *peer) {
	// No channel handling here because peers do not need to block on inv
	// messages.
	if atomic.LoadInt32(&om.shutdown) != 0 {
		return
	}

	om.msgChan <- &invMsg{inv: inv, peer: p}
}

// DonePeer informs the blockmanager that a peer has disconnected.
func (om *objectManager) DonePeer(p *peer) {
	// Ignore if we are shutting down.
	if atomic.LoadInt32(&om.shutdown) != 0 {
		return
	}

	om.msgChan <- &donePeerMsg{peer: p}
}

// Start begins the core block handler which processes block and inv messages.
func (om *objectManager) Start() {
	// Already started?
	if atomic.AddInt32(&om.started, 1) != 1 {
		return
	}

	om.wg.Add(1)
	go om.objectHandler()
}

// Stop gracefully shuts down the block manager by stopping all asynchronous
// handlers and waiting for them to finish.
func (om *objectManager) Stop() error {
	if atomic.AddInt32(&om.shutdown, 1) != 1 {
		return nil
	}

	close(om.quit)
	om.wg.Wait()
	return nil
}

// Pause pauses the block manager until the returned channel is closed.
//
// Note that while paused, all peer and block processing is halted.  The
// message sender should avoid pausing the block manager for long durations.
func (om *objectManager) Pause() chan<- struct{} {
	c := make(chan struct{})
	om.msgChan <- pauseMsg{c}
	return c
}

// newObjectManager returns a new bitcoin block manager.
// Use Start to begin processing asynchronous block and inv updates.
func newObjectManager(s *server, MaxPeers uint) (*objectManager, error) {

	bm := objectManager{
		server:           s,
		requestedObjects: make(map[wire.ShaHash]*peer),
		msgChan:          make(chan interface{}, MaxPeers*3),
		quit:             make(chan struct{}),
	}

	return &bm, nil
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

// setupDB loads (or creates when needed) the block database taking into
// account the selected database backend.  It also contains additional logic
// such warning the user if there are multiple databases which consume space on
// the file system and ensuring the regression test database is clean when in
// regression test mode.
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