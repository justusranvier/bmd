// Originally derived from: btcsuite/btcd/addrmgr/addrmanager_test.go
// Copyright (c) 2013-2015 Conformal Systems LLC.

// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package addrmgr_test

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/DanielKrawisz/bmutil/wire"

	"github.com/DanielKrawisz/bmd/addrmgr"
)

// naTest is used to describe a test to be perfomed against the NetAddressKey
// method.
type naTest struct {
	in   wire.NetAddress
	want string
}

// naTests houses all of the tests to be performed against the NetAddressKey
// method.
var naTests = make([]naTest, 0)

// Put some IP in here for convenience. Points to google.
var someIP = "173.194.115.66"

// addNaTests
func addNaTests() {
	// IPv4
	// Localhost
	addNaTest("127.0.0.1", 8333, "127.0.0.1:8333")
	addNaTest("127.0.0.1", 8334, "127.0.0.1:8334")

	// Class A
	addNaTest("1.0.0.1", 8333, "1.0.0.1:8333")
	addNaTest("2.2.2.2", 8334, "2.2.2.2:8334")
	addNaTest("27.253.252.251", 8335, "27.253.252.251:8335")
	addNaTest("123.3.2.1", 8336, "123.3.2.1:8336")

	// Private Class A
	addNaTest("10.0.0.1", 8333, "10.0.0.1:8333")
	addNaTest("10.1.1.1", 8334, "10.1.1.1:8334")
	addNaTest("10.2.2.2", 8335, "10.2.2.2:8335")
	addNaTest("10.10.10.10", 8336, "10.10.10.10:8336")

	// Class B
	addNaTest("128.0.0.1", 8333, "128.0.0.1:8333")
	addNaTest("129.1.1.1", 8334, "129.1.1.1:8334")
	addNaTest("180.2.2.2", 8335, "180.2.2.2:8335")
	addNaTest("191.10.10.10", 8336, "191.10.10.10:8336")

	// Private Class B
	addNaTest("172.16.0.1", 8333, "172.16.0.1:8333")
	addNaTest("172.16.1.1", 8334, "172.16.1.1:8334")
	addNaTest("172.16.2.2", 8335, "172.16.2.2:8335")
	addNaTest("172.16.172.172", 8336, "172.16.172.172:8336")

	// Class C
	addNaTest("193.0.0.1", 8333, "193.0.0.1:8333")
	addNaTest("200.1.1.1", 8334, "200.1.1.1:8334")
	addNaTest("205.2.2.2", 8335, "205.2.2.2:8335")
	addNaTest("223.10.10.10", 8336, "223.10.10.10:8336")

	// Private Class C
	addNaTest("192.168.0.1", 8333, "192.168.0.1:8333")
	addNaTest("192.168.1.1", 8334, "192.168.1.1:8334")
	addNaTest("192.168.2.2", 8335, "192.168.2.2:8335")
	addNaTest("192.168.192.192", 8336, "192.168.192.192:8336")

	// IPv6
	// Localhost
	addNaTest("::1", 8333, "[::1]:8333")
	addNaTest("fe80::1", 8334, "[fe80::1]:8334")

	// Link-local
	addNaTest("fe80::1:1", 8333, "[fe80::1:1]:8333")
	addNaTest("fe91::2:2", 8334, "[fe91::2:2]:8334")
	addNaTest("fea2::3:3", 8335, "[fea2::3:3]:8335")
	addNaTest("feb3::4:4", 8336, "[feb3::4:4]:8336")

	// Site-local
	addNaTest("fec0::1:1", 8333, "[fec0::1:1]:8333")
	addNaTest("fed1::2:2", 8334, "[fed1::2:2]:8334")
	addNaTest("fee2::3:3", 8335, "[fee2::3:3]:8335")
	addNaTest("fef3::4:4", 8336, "[fef3::4:4]:8336")

	// Tor
	addNaTest("fd87:d87e:eb43::a1", 8333, "aaaaaaaaaaaaaafb.onion:8333")
	addNaTest("fd87:d87e:eb43:cd01::bf32:207", 8334, "zuaqaaaaac7teaqh.onion:8334")
	addNaTest("fd87:d87e:eb43::", 8335, "aaaaaaaaaaaaaaaa.onion:8335")
	addNaTest("fd87:d87e:eb43:ffff::ffff", 8336, "777qaaaaaaaab777.onion:8336")
}

func newNetAddress(ip string) *wire.NetAddress {
	return &wire.NetAddress{
		Timestamp: time.Now(),
		Services:  wire.SFNodeNetwork,
		IP:        net.ParseIP(ip),
		Port:      8333,
	}
}

func randomIPv4Address() *wire.NetAddress {
	return &wire.NetAddress{
		Timestamp: time.Now(),
		Services:  wire.SFNodeNetwork,
		IP: net.IPv4(
			byte(rand.Intn(256)), byte(rand.Intn(256)), byte(rand.Intn(256)), byte(rand.Intn(256))),
		Port: 8333,
	}
}

// genAddresses makes a list of numAddr random IPv4 addresses.
func genAddresses(numAddr int) []*wire.NetAddress {
	addrs := make([]*wire.NetAddress, numAddr)
	for i := 0; i < numAddr; i++ {
		addrs[i] = randomIPv4Address()
	}

	return addrs
}

// genAddressesInSequence makes a list of numAddr IPv4 addresses
// in sequence. All of these addresses will be put in the same bucket.
func genAddressesInSequence(a, b, c, numAddr byte) []*wire.NetAddress {
	addrs := make([]*wire.NetAddress, numAddr)
	var i byte
	for i = 0; i < numAddr; i++ {
		addrs[i] = newNetAddress(fmt.Sprint(a, ".", b, ".", c, ".", i))
	}

	return addrs
}

func addNaTest(ip string, port uint16, want string) {
	nip := net.ParseIP(ip)
	na := wire.NetAddress{
		Timestamp: time.Now(),
		Services:  wire.SFNodeNetwork,
		IP:        nip,
		Port:      port,
	}
	test := naTest{na, want}
	naTests = append(naTests, test)
}

func lookupFunc(host string) ([]net.IP, error) {
	return nil, errors.New("not implemented")
}

// mockLookupFunc takes a map of hosts to ip addresses and returns
// a lookup function for testing purposes.
func mockLookupFunc(m map[string][]net.IP) func(string) ([]net.IP, error) {
	return func(host string) ([]net.IP, error) {
		ip := m[host]
		if ip == nil {
			return ip, errors.New("unknown host")
		} else {
			return ip, nil
		}
	}
}

// TestDeserializeNetAddress ensures that DeserializeNetAddress
// gives errors for invalid inputs.
func TestDeserializeNetAddress(t *testing.T) {
	var tests = []struct {
		input string //The input string.
		err   bool   //Whether an error is returned.
	}{
		{
			"spoon",
			true,
		},
		{
			"google.com:wxyz",
			true,
		},
		{
			"google.com:1234",
			false,
		},
	}

	n := addrmgr.New("",
		mockLookupFunc(map[string][]net.IP{
			"google.com": []net.IP{net.ParseIP("23.34.45.56")}}))

	for _, test := range tests {
		if _, err := n.DeserializeNetAddress(test.input); (err == nil) == test.err {
			t.Errorf("TestDeserializeNetAddress failed for input %s with error %s", test.input, err)
		}
	}
}

func TestStartStop(t *testing.T) {
	n := addrmgr.New("teststartstop", lookupFunc)
	n.Start()
	err := n.Stop()
	if err != nil {
		t.Fatalf("Address Manager failed to stop: %v", err)
	}
	// Stop can be called a second time without causing bad behavior.
	err = n.Stop()
	if err != nil {
		t.Fatalf("Address Manager failed to stop: %v", err)
	}

	n = addrmgr.New("teststartstop", lookupFunc)
	n.Start()
	// Start can be called a second time without causing bad behavior.
	n.Start()
	err = n.Stop()
	if err != nil {
		t.Fatalf("Address Manager failed to stop: %v", err)
	}
}

func TestAddAddressByIP(t *testing.T) {
	fmtErr := fmt.Errorf("")
	addrErr := &net.AddrError{}
	var tests = []struct {
		addrIP string
		err    error
	}{
		{
			someIP + ":8333",
			nil,
		},
		{
			someIP,
			addrErr,
		},
		{
			someIP[:12] + ":8333",
			fmtErr,
		},
		{
			someIP + ":abcd",
			fmtErr,
		},
	}

	amgr := addrmgr.New("testaddressbyip", nil)
	for i, test := range tests {
		err := amgr.AddAddressByIP(test.addrIP)
		if test.err != nil && err == nil {
			t.Errorf("TestGood test %d failed expected an error and got none", i)
			continue
		}
		if test.err == nil && err != nil {
			t.Errorf("TestGood test %d failed expected no error and got one", i)
			continue
		}
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("TestGood test %d failed got %v, want %v", i,
				reflect.TypeOf(err), reflect.TypeOf(test.err))
			continue
		}
	}
}

func TestAddLocalAddress(t *testing.T) {
	var tests = []struct {
		address  wire.NetAddress
		priority addrmgr.AddressPriority
		valid    bool
	}{
		{
			wire.NetAddress{IP: net.ParseIP("192.168.0.100")},
			addrmgr.InterfacePrio,
			false,
		},
		{
			wire.NetAddress{IP: net.ParseIP("204.124.1.1")},
			addrmgr.InterfacePrio,
			true,
		},
		{
			wire.NetAddress{IP: net.ParseIP("204.124.1.1")},
			addrmgr.BoundPrio,
			true,
		},
		{
			wire.NetAddress{IP: net.ParseIP("::1")},
			addrmgr.InterfacePrio,
			false,
		},
		{
			wire.NetAddress{IP: net.ParseIP("fe80::1")},
			addrmgr.InterfacePrio,
			false,
		},
		{
			wire.NetAddress{IP: net.ParseIP("2620:100::1")},
			addrmgr.InterfacePrio,
			true,
		},
	}
	amgr := addrmgr.New("testaddlocaladdress", nil)
	for x, test := range tests {
		result := amgr.AddLocalAddress(&test.address, test.priority)
		if result == nil && !test.valid {
			t.Errorf("TestAddLocalAddress test #%d failed: %s should have "+
				"been accepted", x, test.address.IP)
			continue
		}
		if result != nil && test.valid {
			t.Errorf("TestAddLocalAddress test #%d failed: %s should not have "+
				"been accepted", x, test.address.IP)
			continue
		}
	}
}

func TestAttempt(t *testing.T) {
	n := addrmgr.New("testattempt", lookupFunc)

	// Try an address that has not been added. The function should
	// return without making an attempt.
	na := newNetAddress(someIP)
	n.Attempt(na)
	if n.NumAddresses() != 0 {
		t.Errorf("Address manager should be empty.")
	}

	// Add a new address and get it
	err := n.AddAddressByIP(someIP + ":8333")
	if err != nil {
		t.Fatalf("Adding address failed: %v", err)
	}
	ka := n.GetAddress("any")

	if !ka.LastAttempt().IsZero() {
		t.Errorf("Address should not have attempts, but does")
	}

	na = ka.NetAddress()
	n.Attempt(na)

	if ka.LastAttempt().IsZero() {
		t.Errorf("Address should have an attempt, but does not")
	}
}

func TestConnected(t *testing.T) {
	n := addrmgr.New("testconnected", lookupFunc)

	// Test an address that has not been added. The function should
	// return without attempting to mark the address as connected.
	na := newNetAddress(someIP)
	n.Connected(na)
	if n.NumAddresses() != 0 {
		t.Errorf("Address manager should be empty.")
	}

	// Add a new address and get it
	err := n.AddAddressByIP(someIP + ":8333")
	if err != nil {
		t.Fatalf("Adding address failed: %v", err)
	}
	ka := n.GetAddress("any")
	na = ka.NetAddress()
	na.Timestamp = time.Now().Add(time.Hour * -1) // make it an hour ago

	n.Connected(na)

	if !ka.NetAddress().Timestamp.After(na.Timestamp) {
		t.Errorf("Address should have a new timestamp, but does not")
	}
}

func TestNeedMoreAddresses(t *testing.T) {
	n := addrmgr.New("testneedmoreaddresses", lookupFunc)
	addrsToAdd := 1500
	b := n.NeedMoreAddresses()
	if b == false {
		t.Errorf("Expected that we need more addresses")
	}
	addrs := make([]*wire.NetAddress, addrsToAdd)

	var err error
	for i := 0; i < addrsToAdd; i++ {
		s := fmt.Sprintf("%d.%d.173.147:8333", i/128+60, i%128+60)
		addrs[i], err = n.DeserializeNetAddress(s)
		if err != nil {
			t.Errorf("Failed to turn %s into an address: %v", s, err)
		}
	}

	srcAddr := newNetAddress("173.144.173.111")

	n.AddAddresses(addrs, srcAddr)
	numAddrs := n.NumAddresses()
	if numAddrs > addrsToAdd {
		t.Errorf("Number of addresses is too many: got %d, want %d", numAddrs, addrsToAdd)
	}

	b = n.NeedMoreAddresses()
	if b == true {
		t.Errorf("Expected that we don't need more addresses")
	}
}

func TestAddressCache(t *testing.T) {
	n := addrmgr.New("testaddrcache", lookupFunc)
	srcAddr := newNetAddress("173.144.173.111")
	var cache []*wire.NetAddress

	// No addresses have been added.
	cache = n.AddressCache()
	if cache != nil {
		if len(cache) != 0 {
			t.Error("Address cache should be nil or empty.")
		}
	}

	// Some addresses have been added.
	numAddr := 137
	addrs := genAddresses(numAddr)
	n.TstAddAddressesSkipChecks(addrs, srcAddr)
	expected := n.NumAddresses() * addrmgr.TstGetAddrPercent() / 100
	cache = n.AddressCache()
	if cache == nil {
		t.Error("Address cache should not be nil.")
	} else if len(cache) != expected {
		t.Errorf("Address cache should contain %d addresses but it has %d.", expected, len(cache))
	}

	// More than getAddrMax addresses have been added.
	numAddr = 5 * addrmgr.TstGetAddrMax()
	addrs = genAddresses(numAddr)
	n.TstAddAddressesSkipChecks(addrs, srcAddr)
	expected = addrmgr.TstGetAddrMax()
	cache = n.AddressCache()
	if cache == nil {
		t.Error("Address cache should not be nil.")
	} else if len(cache) != expected {
		t.Errorf("Address cache should contain %d addresses but it has %d.", expected, len(cache))
	}
}

func TestHostToNetAddress(t *testing.T) {
	n := addrmgr.New("",
		mockLookupFunc(map[string][]net.IP{
			"google.com":             []net.IP{net.ParseIP("23.34.45.56")},
			"facebook.com":           []net.IP{},
			"abcdefghijklmnop.onion": []net.IP{net.ParseIP("11.33.55.77")},
			"89abcdefghijklmn.onion": []net.IP{net.ParseIP("77.66.55.44")}}))

	var tests = []struct {
		input string // The input string.
		err   bool   // Whether an error is returned.
	}{
		{
			"google.com",
			false,
		},
		{
			"facebook.com",
			true,
		},
		{
			"abcdefghijklmnop.onion",
			false,
		},
		{
			"twitter.com",
			true,
		},
		{
			"89abcdefghijklmn.onion",
			true,
		},
	}

	for _, test := range tests {
		if _, err := n.HostToNetAddress(test.input, uint16(1234), uint32(1), wire.SFNodeNetwork); (err == nil) == test.err {
			t.Errorf("TestDeserializeNetAddress failed for input \"%s\" with error \"%s\"", test.input, err)
		}
	}
}

func TestGood(t *testing.T) {
	n := addrmgr.New("testgood", lookupFunc)
	addrsToAdd := 64 * 64
	addrs := make([]*wire.NetAddress, addrsToAdd)

	var err error
	for i := 0; i < addrsToAdd; i++ {
		s := fmt.Sprintf("%d.173.147.%d:8333", i/64+60, i%64+60)
		addrs[i], err = n.DeserializeNetAddress(s)
		if err != nil {
			t.Errorf("Failed to turn %s into an address: %v", s, err)
		}
	}

	srcAddr := newNetAddress("173.144.173.111")

	n.AddAddresses(addrs, srcAddr)
	for _, addr := range addrs {
		n.Good(addr)
	}

	numAddrs := n.NumAddresses()
	if numAddrs >= addrsToAdd {
		t.Errorf("Number of addresses is too many: %d vs %d", numAddrs, addrsToAdd)
	}

	numCache := len(n.AddressCache())
	if numCache >= numAddrs/4 {
		t.Errorf("Number of addresses in cache: got %d, want %d", numCache, numAddrs/4)
	}
}

// TestGoodEdgeCases tests some unusual control paths in Good.
func TestGoodEdgeCases(t *testing.T) {
	n := addrmgr.New("testgood", lookupFunc)
	netAddr := newNetAddress("9.8.7.6")
	ka := addrmgr.TstNewKnownAddress(netAddr,
		0, time.Now().Add(-30*time.Minute), time.Now(), false, 0)
	srcAddr := newNetAddress("173.144.173.111")

	// Set an address as good and do it again.
	n.TstAddKnownAddress(ka, 4)
	n.Good(netAddr)
	bucket_before, tried_before := n.TstGetBucketAndTried(ka)
	n.Good(netAddr)
	bucket_after, tried_after := n.TstGetBucketAndTried(ka)
	// Address should not have changed positions.
	if len(bucket_before) != len(bucket_after) ||
		bucket_before[0] != bucket_after[0] ||
		tried_before != tried_after {
		t.Error("An address which is set as good a second time in a row should not be changed.")
	}
	// There should be no other addreses either.
	if n.NumAddresses() != 1 {
		t.Error("There should be exactly one address.")
	}

	// Set up an illegal situation where an address is registered
	// but not in any new bucket.
	n = addrmgr.New("testgood", lookupFunc)
	n.TstAddAddressNoBucket(netAddr, srcAddr)
	n.Good(netAddr)
	// The address should have been removed.
	if n.NumAddresses() != 0 {
		t.Errorf("Did not recover from illegal state. NumAddresses should be zero, got %d.", n.NumAddresses())
	}

	// Set up a situation in which there is no room in an old
	// bucket.
	n = addrmgr.New("testgood", lookupFunc)

	// Make a bunch of addresses, mark them as good and put them
	// all in a particular bucket.
	var numAddr byte = 75
	triedBucket := n.TstGetTriedBucket(netAddr)
	addrs := genAddressesInSequence(13, 14, 15, numAddr)
	n.TstAddAddressesSkipChecks(addrs, srcAddr)
	for _, addr := range addrs {
		n.TstGoodNoChecks(addr, triedBucket)
	}

	// Add in more addresses to the new bucket. Now there are 75 in each.
	addrs = genAddressesInSequence(13, 14, 16, numAddr)
	n.TstAddAddressesSkipChecks(addrs, srcAddr)

	// Now create a new address to try to mark as good.
	n.TstAddKnownAddress(
		addrmgr.TstNewKnownAddress(netAddr,
			0, time.Now().Add(-30*time.Minute), time.Now(), false, 0), 5)

	// Move the addresses to the tried bucket.
	n.Good(netAddr)

	if _, tried := n.TstGetBucketAndTried(ka); !tried {
		t.Errorf("Address should have been marked as tried.")
	}
}

func TestGetAddress(t *testing.T) {
	var n *addrmgr.AddrManager
	var ka *addrmgr.KnownAddress

	n = addrmgr.New("testgetaddress", lookupFunc)

	// Get an address from an empty set (should error)
	if rv := n.GetAddress("any"); rv != nil {
		t.Errorf("GetAddress failed: got: %v want: %v\n", rv, nil)
	}

	// Add a new address and get it
	err := n.AddAddressByIP(someIP + ":8333")
	if err != nil {
		t.Fatalf("Adding address failed: %v", err)
	}
	ka = n.GetAddress("any")
	if ka == nil {
		t.Fatalf("Did not get an address where there is one in the pool")
	}
	if ka.NetAddress().IP.String() != someIP {
		t.Errorf("Wrong IP: got %v, want %v", ka.NetAddress().IP.String(), someIP)
	}

	// Mark this as a good address and get it
	n.Good(ka.NetAddress())
	ka = n.GetAddress("any")
	if ka == nil {
		t.Fatalf("Did not get an address where there is one in the pool")
	}
	if ka.NetAddress().IP.String() != someIP {
		t.Errorf("Wrong IP: got %v, want %v", ka.NetAddress().IP.String(), someIP)
	}

	numAddrs := n.NumAddresses()
	if numAddrs != 1 {
		t.Errorf("Wrong number of addresses: got %d, want %d", numAddrs, 1)
	}

	// Start over to test a few weird cases.
	n = addrmgr.New("testgetaddress", lookupFunc)
	ipA := "9.8.7.6"
	ipB := "98.76.54.32"
	netAddrA := newNetAddress(ipA)
	netAddrB := newNetAddress(ipB)

	n.TstAddKnownAddress(
		addrmgr.TstNewKnownAddress(netAddrA,
			28, time.Now().Add(-30*time.Minute), time.Now(), false, 0), 3)
	n.TstAddKnownAddress(
		addrmgr.TstNewKnownAddress(netAddrB,
			10, time.Now().Add(-30*time.Minute), time.Now(), false, 0), 3)
	ka = n.GetAddress("any")

	if ka == nil {
		t.Fatalf("Did not get an address where there is one in the pool")
	}
	// Note: this test will occaisionally fail with a low probability. There
	// is a small probability of selecting the incorrect address, but that is
	// normal behavior for the program.
	if ka.NetAddress().IP.String() != ipB {
		t.Errorf("Wrong IP: got %v, want %v", ka.NetAddress().IP.String(), ipB)
	}

	n.TstGoodNoChecks(netAddrA, 3)
	n.TstGoodNoChecks(netAddrB, 3)

	ka = n.GetAddress("any")
	if ka == nil {
		t.Fatalf("Did not get an address where there is one in the pool")
	}
	if ka.NetAddress().IP.String() != ipB {
		t.Errorf("Wrong IP: got %v, want %v", ka.NetAddress().IP.String(), ipB)
	}
}

func TestGetBestLocalAddress(t *testing.T) {
	localAddrs := []wire.NetAddress{
		{IP: net.ParseIP("192.168.0.100")},
		{IP: net.ParseIP("::1")},
		{IP: net.ParseIP("fe80::1")},
		{IP: net.ParseIP("2001:470::1")},
	}

	var tests = []struct {
		remoteAddr wire.NetAddress
		want0      wire.NetAddress
		want1      wire.NetAddress
		want2      wire.NetAddress
		want3      wire.NetAddress
	}{
		{
			// Remote connection from public IPv4
			wire.NetAddress{IP: net.ParseIP("204.124.8.1")},
			wire.NetAddress{IP: net.IPv4zero},
			wire.NetAddress{IP: net.IPv4zero},
			wire.NetAddress{IP: net.ParseIP("204.124.8.100")},
			wire.NetAddress{IP: net.ParseIP("fd87:d87e:eb43:25::1")},
		},
		{
			// Remote connection from private IPv4
			wire.NetAddress{IP: net.ParseIP("172.16.0.254")},
			wire.NetAddress{IP: net.IPv4zero},
			wire.NetAddress{IP: net.IPv4zero},
			wire.NetAddress{IP: net.IPv4zero},
			wire.NetAddress{IP: net.IPv4zero},
		},
		{
			// Remote connection from public IPv6
			wire.NetAddress{IP: net.ParseIP("2602:100:abcd::102")},
			wire.NetAddress{IP: net.IPv6zero},
			wire.NetAddress{IP: net.ParseIP("2001:470::1")},
			wire.NetAddress{IP: net.ParseIP("2001:470::1")},
			wire.NetAddress{IP: net.ParseIP("2001:470::1")},
		},
		/* XXX
		{
			// Remote connection from Tor
			wire.NetAddress{IP: net.ParseIP("fd87:d87e:eb43::100")},
			wire.NetAddress{IP: net.IPv4zero},
			wire.NetAddress{IP: net.ParseIP("204.124.8.100")},
			wire.NetAddress{IP: net.ParseIP("fd87:d87e:eb43:25::1")},
		},
		*/
	}

	amgr := addrmgr.New("testgetbestlocaladdress", nil)

	// Test against default when there's no address
	for x, test := range tests {
		got := amgr.GetBestLocalAddress(&test.remoteAddr)
		if !test.want0.IP.Equal(got.IP) {
			t.Errorf("TestGetBestLocalAddress test1 #%d failed for remote address %s: want %s got %s",
				x, test.remoteAddr.IP, test.want1.IP, got.IP)
			continue
		}
	}

	for _, localAddr := range localAddrs {
		amgr.AddLocalAddress(&localAddr, addrmgr.InterfacePrio)
	}

	// Test against want1
	for x, test := range tests {
		got := amgr.GetBestLocalAddress(&test.remoteAddr)
		if !test.want1.IP.Equal(got.IP) {
			t.Errorf("TestGetBestLocalAddress test1 #%d failed for remote address %s: want %s got %s",
				x, test.remoteAddr.IP, test.want1.IP, got.IP)
			continue
		}
	}

	// Add a public IP to the list of local addresses.
	localAddr := wire.NetAddress{IP: net.ParseIP("204.124.8.100")}
	amgr.AddLocalAddress(&localAddr, addrmgr.InterfacePrio)

	// Test against want2
	for x, test := range tests {
		got := amgr.GetBestLocalAddress(&test.remoteAddr)
		if !test.want2.IP.Equal(got.IP) {
			t.Errorf("TestGetBestLocalAddress test2 #%d failed for remote address %s: want %s got %s",
				x, test.remoteAddr.IP, test.want2.IP, got.IP)
			continue
		}
	}
	/*
		// Add a tor generated IP address
		localAddr = wire.NetAddress{IP: net.ParseIP("fd87:d87e:eb43:25::1")}
		amgr.AddLocalAddress(&localAddr, addrmgr.ManualPrio)

		// Test against want3
		for x, test := range tests {
			got := amgr.GetBestLocalAddress(&test.remoteAddr)
			if !test.want3.IP.Equal(got.IP) {
				t.Errorf("TestGetBestLocalAddress test3 #%d failed for remote address %s: want %s got %s",
					x, test.remoteAddr.IP, test.want3.IP, got.IP)
				continue
			}
		}
	*/
}

func TestNetAddressKey(t *testing.T) {
	addNaTests()

	t.Logf("Running %d tests", len(naTests))
	for i, test := range naTests {
		key := addrmgr.NetAddressKey(&test.in)
		if key != test.want {
			t.Errorf("NetAddressKey #%d\n got: %s want: %s", i, key, test.want)
			continue
		}
	}

}

func TestGetReachabilityFrom(t *testing.T) {
	const (
		Unreachable = 0
		Default     = iota
		Teredo
		Ipv6Weak
		Ipv4
		Ipv6Strong
		Private
	)

	categories := map[int]string{
		Unreachable: "Unreachable",
		Default:     "Default",
		Teredo:      "Teredo",
		Ipv6Weak:    "Ipv6Weak",
		Ipv4:        "Ipv4",
		Ipv6Strong:  "Ipv6Strong",
		Private:     "Private",
	}

	test_addresses := []string{
		"192.168.0.1",        // Unroutable address
		"172.32.1.1",         // Regular IPv4 address
		"::ffff:abcd:ef12:1", // Regular IPv6 address
		"2001::1",            // RFC4380
		"64:ff9b::1",         // Tunnelled IPv6
		"fd87:d87e:eb43::a1", // Onion address.
	}

	expected := [][]int{
		[]int{Unreachable, Unreachable, Default, Default, Default, Default},
		[]int{Unreachable, Ipv4, Ipv4, Ipv4, Ipv4, Ipv4},
		[]int{Unreachable, Unreachable, Ipv6Strong, Ipv6Weak, Ipv6Strong, Default},
		[]int{Unreachable, Unreachable, Teredo, Teredo, Teredo, Default},
		[]int{Unreachable, Unreachable, Ipv6Weak, Ipv6Weak, Ipv6Weak, Default},
		[]int{Unreachable, Unreachable, Ipv6Strong, Ipv6Weak, Ipv6Strong, Private},
	}

	for i := 0; i < len(test_addresses); i++ {
		if i >= len(expected) {
			break
		}

		for j := 0; j < len(test_addresses); j++ {
			if j >= len(expected[i]) {
				break
			}

			reachability := addrmgr.TstGetReachabilityFrom(
				newNetAddress(test_addresses[i]), newNetAddress(test_addresses[j]))
			if reachability != expected[i][j] {
				t.Errorf("Error routing %s to %s. Got %s expected %s",
					test_addresses[i], test_addresses[j],
					categories[reachability], categories[expected[i][j]])
			}
		}
	}
}
