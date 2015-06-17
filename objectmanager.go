// Originally derived from: btcsuite/btcd/blockmanager.go
// Copyright (c) 2013-2015 the btcsuite developers.

// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/list"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/monetas/bmd/database"
	_ "github.com/monetas/bmd/database/memdb"
	"github.com/monetas/bmutil/pow"
	"github.com/monetas/bmutil/wire"
)

const (
	// objectManagerQueueSize specifies the size of object manager msgChan.
	objectManagerQueueSize = 50

	// objectDbNamePrefix is the prefix for the object database name. The
	// database type is appended to this value to form the full object database
	// name.
	objectDbNamePrefix = "objects"

	// MaxPeerRequests is the maximum number of outstanding object requests a
	// may have at any given time.
	MaxPeerRequests = 120
)

// newPeerMsg signifies a newly connected peer to the object manager.
type newPeerMsg struct {
	peer *bmpeer
}

// objectMsg packages a bitmessage object message and the peer it came from
// together so the object manager has access to that information.
type objectMsg struct {
	object *wire.MsgObject
	peer   *bmpeer
}

// invMsg packages a bitmessage inv message and the peer it came from together
// so the object manager has access to that information.
type invMsg struct {
	inv  *wire.MsgInv
	peer *bmpeer
}

// donePeerMsg signifies a newly disconnected peer to the object manager.
type donePeerMsg struct {
	peer *bmpeer
}

// peerRequest represents the peer from which an object was requested along with
// the timestamp.
type peerRequest struct {
	peer      *bmpeer
	timestamp time.Time
}

// readyPeerMsg signals that a peer is ready to download more objects.
type readyPeerMsg bmpeer

// ObjectManager provides a concurrency safe object manager for handling all
// incoming and outgoing.
type ObjectManager struct {
	server   *server
	started  int32
	shutdown int32

	// The set of peers that are fully connected.
	peers map[*bmpeer]struct{}

	// The set of peers that are working on downloading messages.
	working map[*bmpeer]struct{}

	// The set of objects which we do not have and which have not been
	// assigned to peers for download.
	unknown map[wire.InvVect]struct{}

	// The set of objects which have been assigned to peers for download.
	requested map[wire.InvVect]*peerRequest

	relayInvChan chan *wire.InvVect
	relayInvList *list.List
	msgChan      chan interface{}
	wg           sync.WaitGroup
	quit         chan struct{}
}

// handleNewPeer deals with new peers that have signalled they may
// be considered as a sync peer (they have already successfully negotiated). It
// is invoked from the syncHandler goroutine.
func (om *ObjectManager) handleNewPeer(p *bmpeer) {
	// Ignore if in the process of shutting down.
	if atomic.LoadInt32(&om.shutdown) != 0 {
		return
	}

	// Add the peer
	om.peers[p] = struct{}{}
	objmgrLog.Debug("Peer ", p.addr.String(), " added to object manager.")
}

// handleDonePeer deals with peers that have signalled they are done. It is
// invoked from the syncHandler goroutine.
func (om *ObjectManager) handleDonePeer(p *bmpeer) {
	if _, ok := om.peers[p]; !ok {
		return
	}

	// Remove the peer from the list of candidate peers.
	delete(om.peers, p)
	delete(om.working, p)

	reassignInvs := 0
	// Remove requested objects from the global map so that they will be fetched
	// from elsewhere next time we get an inv.
	for invHash, objPeer := range om.requested {
		if objPeer.peer == p { // peer matches
			delete(om.requested, invHash)

			om.unknown[invHash] = struct{}{}

			reassignInvs++
		}
	}

	objmgrLog.Debug("Peer ", p.addr.String(), " removed from object manager; ", reassignInvs, " reassigned invs.")
	if reassignInvs > 0 {
		om.assignRequests()
	}
}

// handleObjectMsg handles object messages from all peers.
func (om *ObjectManager) handleObjectMsg(omsg *objectMsg) {
	invVect := wire.NewInvVect(omsg.object.InventoryHash())

	// Unrequested data means disconnect.
	if p, exists := om.requested[*invVect]; !exists || p.peer != omsg.peer {
		// An attacker could guess which objects are being requested from peers
		// and send them before the actual peer the object was requested from,
		// thus disconnecting legitimate peers. We want to prevent against such
		// an attack by checking that objects came from peers that we requested
		// from.
		objmgrLog.Error(omsg.peer.addr.String(),
			" Disconnecting because of unrequested object ", invVect.Hash.String()[:8], " received.")
		omsg.peer.disconnect()
		return
	}

	// TODO Before, when this number was not logged, numbers could be recorded
	// that were off by a long enough time to cause expired object requests.
	// There must be some way to get it working right, but it seems to work
	// as long as I log the time!
	now := time.Now()
	omsg.peer.lastReceipt = time.Now()

	delete(om.requested, *invVect)

	// Check PoW.
	if !pow.Check(omsg.object, pow.DefaultExtraBytes,
		pow.DefaultNonceTrialsPerByte, time.Now()) {
		return // invalid PoW
	}

	objmgrLog.Debugf(omsg.peer.peer.PrependAddr(fmt.Sprint("Object ", invVect.Hash.String()[:8],
		" received; ", omsg.peer.inventory.NumRequests(), " still assigned; ", len(om.requested), " still queued; ",
		len(om.unknown), " unqueued; last receipt = ", now)))

	om.handleInsert(omsg.object)
}

func (om *ObjectManager) handleInsert(obj *wire.MsgObject) uint64 {
	invVect := wire.NewInvVect(obj.InventoryHash())
	// Insert object into database.
	counter, err := om.server.db.InsertObject(obj)
	if err != nil {
		dbLog.Errorf("failed to insert object: %v", err)
		return 0
	}

	// Notify RPC server
	if !cfg.DisableRPC {
		om.server.rpcServer.NotifyObject(obj, counter)
	}

	// Advertise objects to other peers.
	om.relayInvList.PushBack(invVect)

	return counter
}

// haveInventory returns whether or not the inventory represented by the passed
// inventory vector is known. This includes checking all of the various places
// inventory can be.
func (om *ObjectManager) haveInventory(invVect *wire.InvVect) (bool, error) {
	return om.server.db.ExistsObject(&invVect.Hash)
}

// handleInvMsg handles inv messages from all peers.
// We examine the inventory advertised by the remote peer and act accordingly.
func (om *ObjectManager) handleInvMsg(imsg *invMsg) {
	requestList := make([]*wire.InvVect, len(imsg.inv.InvList))

	// Request the advertised inventory if we don't already have it.
	numInvs := 0
	for _, iv := range imsg.inv.InvList {
		// Add inv to known inventory.
		imsg.peer.inventory.AddKnown(iv)

		haveInv, err := om.haveInventory(iv)
		if err != nil || haveInv {
			continue
		}

		// If the object is already known about, ignore it.
		if _, ok := om.unknown[*iv]; ok {
			continue
		}

		// If this has already been requested, ignore it.
		if _, ok := om.requested[*iv]; ok {
			continue
		}

		// Add it to the list of invs to be requested.
		requestList[numInvs] = iv
		numInvs++
	}

	// Nothing to request, so we are done.
	if numInvs == 0 {
		return
	}
	objmgrLog.Trace("Inv received with ", len(requestList), " unknown objects.")

	// If the request can fit, we just request it from the peer that told us
	// about it in the first place.
	if numInvs <= MaxPeerRequests-imsg.peer.inventory.NumRequests() {

		for _, iv := range requestList[:numInvs] {
			om.requested[*iv] = &peerRequest{
				peer:      imsg.peer,
				timestamp: time.Now(),
			}
		}
		objmgrLog.Trace("All are assigned to peer ", imsg.peer.addr.String(),
			"; total assigned is ", len(om.requested))

		imsg.peer.PushGetDataMsg(requestList[:numInvs])
		om.working[imsg.peer] = struct{}{}

		// Send the peer more to download if there is more that it could be doing.
		// This can happen if a peer connects and sends an inv message with only
		// one or two new invs while the initial download is going on, so it
		// needs to be given a lot more to download.
		if len(om.unknown) > 0 {
			om.handleReadyPeer(imsg.peer)
		}
		return
	}

	// Add the new invs to the list of unknown invs.
	for _, iv := range requestList[:numInvs] {
		om.unknown[*iv] = struct{}{}
	}

	// If the number of unknown objects had been zero before, we have to do
	// something to get the download process started again.
	if len(om.unknown) == numInvs {
		om.assignRequests()
	}
}

// assignRequestsAll takes the list of unknown objects and tries to assign
// some to each peer, up to the maximum allowed.
func (om *ObjectManager) assignRequests() {
	if len(om.peers) == 0 {
		return
	}
	objmgrLog.Trace("Assigning objects to peers for download; number of requested objects: ", len(om.requested))
	time.Sleep(5 * time.Second)

	for peer := range om.peers {
		if len(om.unknown) == 0 {
			return
		}

		// Assign some unknown objects to each peer.
		om.handleReadyPeer(peer)
	}
}

func (om *ObjectManager) handleReadyPeer(peer *bmpeer) {
	assigned := 0

	// Detect if no peers are working by the end of this function and call
	// cleanUnknown if so.
	defer func() {
		if assigned != 0 {
			om.working[peer] = struct{}{}
			return
		}
		delete(om.working, peer)
		if len(om.working) == 0 {
			om.cleanUnknown()
		}
	}()

	// If there are no objects to get, we don't need to request anything.
	if len(om.unknown) == 0 {
		return
	}
	objmgrLog.Trace("Assigning objects to peer ", peer.addr.String())

	// If the object is already downloading too much, return.
	max := MaxPeerRequests - peer.inventory.NumRequests()
	if max <= 0 {
		return
	}

	requestList := make([]*wire.InvVect, max)

	// Since unknown is a map, the elements come out randomly ordered. Therefore
	// (for now at least) object request handling does not work like a queue.
	// We might ask for new invs earlier than old invs, or with any order at all.
	//objmgrLog.Trace("Looping through unknown objects. Max is ", max)
	for iv := range om.unknown {
		//objmgrLog.Trace("inv : ", iv.Hash.String())
		if assigned >= max {
			break
		}

		// Add an inv in the list of unknown objects to the request list
		// if the peer knows about it.
		if !peer.inventory.IsKnown(&iv) {
			continue
		}

		if _, ok := om.requested[iv]; ok {
			objmgrLog.Error("Object ", iv.Hash.String()[:8], " is in both requested objects AND unknown objects. Should not happen!")
			delete(om.unknown, iv)
			continue
		}

		// Add to the list of requested objects and remove from the list of unknown objects.
		newiv := iv
		requestList[assigned] = &newiv
		delete(om.unknown, iv)

		om.requested[iv] = &peerRequest{
			peer:      peer,
			timestamp: time.Now(),
		}

		assigned++
	}

	// If there was nothing the peer could give us, return.
	if assigned == 0 {
		return
	}

	objmgrLog.Trace(assigned, " objects assigned to peer ", peer.addr.String(), ". ", len(om.unknown),
		" unassigned objects remaining and ", len(om.requested), " total assigned.")
	peer.PushGetDataMsg(requestList[:assigned])
}

// cleanUnknown is called after all the downloading work is done and it clears
// out any remaining invs that are not known by any peers.
func (om *ObjectManager) cleanUnknown() {
	if len(om.unknown) == 0 {
		return
	}

loop:
	for iv := range om.unknown {
		for peer := range om.peers {
			if peer.inventory.IsKnown(&iv) {
				peer.PushGetDataMsg([]*wire.InvVect{&iv})
				continue loop
			}
		}

		delete(om.unknown, iv)
	}
}

// clearRequests is used to clear out timed out requests periodically. It's used
// to prevent against a scenario in which a malicious peer advertises an inv
// hash but does not send the object. This would effectively 'censor' the object
// from the peer. To avoid this scenario, we need to record the timestamp of a
// request and set it to timeout within the set duration.
func (om *ObjectManager) clearRequests(d time.Duration) {
	now := time.Now()
	// Collect all peers to be disconnected in one map or we lock up the
	// objectHandler with too many requests at once.
	peerDisconnect := make(map[*bmpeer]struct{})
	for hash, p := range om.requested {
		// if request has expired
		if p.timestamp.Add(d).Before(now) {
			peerQueueSize := p.peer.inventory.NumRequests()
			// If the peer has a long queue of requested objects, then don't
			// disconnect it unless it has not received SOME object recently enough,
			// or if the request is expired by a much longer time.
			if peerQueueSize < 10 ||
				p.timestamp.Add(d*time.Duration(peerQueueSize)).Before(now) ||
				p.peer.lastReceipt.Add(d).Before(now) {

				if _, ok := peerDisconnect[p.peer]; !ok {
					objmgrLog.Debug(p.peer.peer.PrependAddr(fmt.Sprint(
						" disconnecting due to expired request for object ", hash.Hash.String()[:8], "; queue size = ",
						peerQueueSize, "; last receipt =", p.peer.lastReceipt, "; cond 1 =",
						p.timestamp.Add(d*time.Duration(peerQueueSize)).Before(now),
						"; cond 2 = ", p.peer.lastReceipt.Add(d).Before(now))))

					peerDisconnect[p.peer] = struct{}{}
				}
			}
		}
	}

	// Disconnect the peers gradually in a separate go routine so as not
	// to deluge the object manager with requests.
	go func() {
		for peer := range peerDisconnect {
			peer.disconnect() // we're done with this malicious peer
			time.Sleep(time.Second)
		}
	}()
}

// handleRelayInvMsg deals with relaying inventory to peers that are not already
// known to have it. It is invoked from the peerHandler goroutine.
func (om *ObjectManager) handleRelayInvMsg(inv []*wire.InvVect) {
	for peer := range om.peers {
		ivl := peer.inventory.FilterKnown(inv)

		if len(ivl) == 0 {
			return
		}

		// Queue the inventory to be relayed with the next batch.
		// It will be ignored if the peer is already known to
		// have the inventory.
		peer.send.QueueInventory(ivl)
	}
}

// objectHandler is the main handler for the object manager. It must be run as a
// goroutine. It processes inv messages in a separate goroutine from the peer
// handlers.
func (om *ObjectManager) objectHandler() {
	clearTick := time.NewTicker(cfg.RequestExpire / 2)
	relayInvTick := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-om.quit:
			relayInvTick.Stop()
			clearTick.Stop()
			om.wg.Done()
			return

		case <-clearTick.C:
			// Under normal operation, we expire requests much more quickly
			// than during the initial download.
			om.clearRequests(cfg.RequestExpire)

		// Relay to the other peers all the new invs collected
		// during the past ten seconds.
		case <-relayInvTick.C:
			objmgrLog.Trace("Relay inv ticker ticked.")
			if om.relayInvList.Len() == 0 {
				continue
			}
			invs := make([]*wire.InvVect, om.relayInvList.Len())
			i := 0
			for e := om.relayInvList.Front(); e != nil; e = e.Next() {
				invs[i] = e.Value.(*wire.InvVect)
				i++
			}
			om.relayInvList = list.New()
			objmgrLog.Trace("Relying list of invs of size ", len(invs))
			om.handleRelayInvMsg(invs)

		case m := <-om.msgChan:
			switch msg := m.(type) {

			case *newPeerMsg:
				om.handleNewPeer(msg.peer)

			case *objectMsg:
				om.handleObjectMsg(msg)

			case *invMsg:
				om.handleInvMsg(msg)

			case *donePeerMsg:
				om.handleDonePeer(msg.peer)

			case *readyPeerMsg:
				om.handleReadyPeer((*bmpeer)(msg))
			}
		}
	}
}

// NewPeer informs the object manager of a newly active peer.
func (om *ObjectManager) NewPeer(p *bmpeer) {
	// Ignore if we are shutting down.
	if atomic.LoadInt32(&om.shutdown) != 0 {
		return
	}

	om.msgChan <- &newPeerMsg{peer: p}
}

// ReadyPeer signals that a peer is ready to download more objects.
func (om *ObjectManager) ReadyPeer(p *bmpeer) {
	// Ignore if we are shutting down.
	if atomic.LoadInt32(&om.shutdown) != 0 {
		return
	}

	om.msgChan <- &p
}

// QueueObject adds the passed object message and peer to the object handling
// queue.
func (om *ObjectManager) QueueObject(object *wire.MsgObject, p *bmpeer) {
	// Don't accept more objects if we're shutting down.
	if atomic.LoadInt32(&om.shutdown) != 0 {
		return
	}

	om.msgChan <- &objectMsg{object: object, peer: p}
}

// QueueInv adds the passed inv message and peer to the object handling queue.
func (om *ObjectManager) QueueInv(inv *wire.MsgInv, p *bmpeer) {
	// Ignore if we are shutting down.
	if atomic.LoadInt32(&om.shutdown) != 0 {
		return
	}

	om.msgChan <- &invMsg{inv: inv, peer: p}
}

// DonePeer informs the object manager that a peer has disconnected.
func (om *ObjectManager) DonePeer(p *bmpeer) {
	// Ignore if we are shutting down.
	if atomic.LoadInt32(&om.shutdown) != 0 {
		return
	}

	om.msgChan <- &donePeerMsg{peer: p}
}

// Start begins the core object handler which processes object messages.
func (om *ObjectManager) Start() {
	// Already started?
	if atomic.AddInt32(&om.started, 1) != 1 {
		return
	}

	objmgrLog.Info("Object manager started.")

	om.wg.Add(1)
	go om.objectHandler()
}

// Stop gracefully shuts down the object manager by stopping all asynchronous
// handlers and waiting for them to finish.
func (om *ObjectManager) Stop() error {
	if atomic.AddInt32(&om.shutdown, 1) != 1 {
		return nil
	}

	objmgrLog.Info("Object manager stopped.")

	close(om.quit)
	om.wg.Wait()
	return nil
}

// newObjectManager returns a new bitmessage object manager. Use Start to begin
// processing objects and inv messages asynchronously.
func newObjectManager(s *server) *ObjectManager {
	return &ObjectManager{
		server:       s,
		requested:    make(map[wire.InvVect]*peerRequest),
		unknown:      make(map[wire.InvVect]struct{}),
		msgChan:      make(chan interface{}),
		quit:         make(chan struct{}),
		relayInvChan: make(chan *wire.InvVect, objectManagerQueueSize),
		peers:        make(map[*bmpeer]struct{}),
		working:      make(map[*bmpeer]struct{}),
		relayInvList: list.New(),
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
