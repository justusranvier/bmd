package addrmgr_test

import (
	"testing"
	"time"
	"math"

	"github.com/monetas/bmd/wire"
	"github.com/monetas/bmd/addrmgr"
)

func TestChance(t *testing.T) {
	var tests = []struct {
		addr *addrmgr.KnownAddress
		expected float64
	} {
		{
			//Test normal case
			addrmgr.NewKnownAddress(&wire.NetAddress{Timestamp: time.Now().Add(-35 * time.Second)},
				0, time.Now().Add(-30 * time.Minute), time.Now(), false, 0),
			.944882,
		}, {
			//Test case in which lastseen < 0
			addrmgr.NewKnownAddress(&wire.NetAddress{Timestamp: time.Now().Add(20 * time.Second)},
				0, time.Now().Add(-30 * time.Minute), time.Now(), false, 0),
			1.000,
		}, {
			//Test case in which lastattempt < 0
			addrmgr.NewKnownAddress(&wire.NetAddress{Timestamp: time.Now().Add(-35 * time.Second)},
				0, time.Now().Add(30 * time.Minute), time.Now(), false, 0),
			.00944882,
		}, {
			//Test case in which lastattempt < ten minutes
			addrmgr.NewKnownAddress(&wire.NetAddress{Timestamp: time.Now().Add(-35 * time.Second)},
				0, time.Now().Add(-5 * time.Minute), time.Now(), false, 0),
			.00944882,
		}, {
			//Test case with several failed attempts. 
			addrmgr.NewKnownAddress(&wire.NetAddress{Timestamp: time.Now().Add(-35 * time.Second)},
				2, time.Now().Add(-30 * time.Minute), time.Now(), false, 0),
			.419948,
		},
	}

	err := .0001
	for i, test := range tests {
		chance := addrmgr.KnownAddressChance(test.addr)
		if math.Abs(test.expected - chance) >= err {
			t.Errorf("case %d: got %f, expected %f", i, chance, test.expected)
		}
	}
}

func TestIsBad(t *testing.T) {
	future := time.Now().Add(35 * time.Minute)
	monthOld := time.Now().Add(-43 * time.Hour * 24)
	secondsOld := time.Now().Add(-2 * time.Second)
	minutesOld := time.Now().Add(-27 * time.Minute)
	hoursOld := time.Now().Add(-5 * time.Hour)
	zeroTime, _ := time.Parse("Jan 1, 1970 at 0:00am (GMT)", "Jan 1, 1970 at 0:00am (GMT)")

	futureNa := &wire.NetAddress{Timestamp: future}
	minutesOldNa := &wire.NetAddress{Timestamp: minutesOld}
	monthOldNa := &wire.NetAddress{Timestamp: monthOld}
	currentNa := &wire.NetAddress{Timestamp: secondsOld}

	//Test addresses that have been tried in the last minute.
	if addrmgr.KnownAddressIsBad(addrmgr.NewKnownAddress(futureNa, 3, secondsOld, zeroTime, false, 0)) {
		t.Errorf("test case 1: addresses that have been tried in the last minute are not bad.")
	}
	if addrmgr.KnownAddressIsBad(addrmgr.NewKnownAddress(monthOldNa, 3, secondsOld, zeroTime, false, 0)) {
		t.Errorf("test case 2: addresses that have been tried in the last minute are not bad.")
	}
	if addrmgr.KnownAddressIsBad(addrmgr.NewKnownAddress(currentNa, 3, secondsOld, zeroTime, false, 0)) {
		t.Errorf("test case 3: addresses that have been tried in the last minute are not bad.")
	}
	if addrmgr.KnownAddressIsBad(addrmgr.NewKnownAddress(currentNa, 3, secondsOld, monthOld, true, 0)) {
		t.Errorf("test case 4: addresses that have been tried in the last minute are not bad.")
	}
	if addrmgr.KnownAddressIsBad(addrmgr.NewKnownAddress(currentNa, 2, secondsOld, secondsOld, true, 0)) {
		t.Errorf("test case 5: addresses that have been tried in the last minute are not bad.")
	}

	//Test address that claims to be from the future.
	if !addrmgr.KnownAddressIsBad(addrmgr.NewKnownAddress(futureNa, 0, minutesOld, hoursOld, true, 0)) {
		t.Errorf("test case 6: addresses that claim to be from the future are bad.")
	}

	//Test address that has not been seen in over a month.
	if !addrmgr.KnownAddressIsBad(addrmgr.NewKnownAddress(monthOldNa, 0, minutesOld, hoursOld, true, 0)) {
		t.Errorf("test case 7: addresses more than a month old are bad.")
	}

	//It has failed at least three times and never succeeded.
	if !addrmgr.KnownAddressIsBad(addrmgr.NewKnownAddress(minutesOldNa, 3, minutesOld, zeroTime, true, 0)) {
		t.Errorf("test case 8: addresses that have never succeeded are bad.")
	}

	//It has failed ten times in the last week
	if !addrmgr.KnownAddressIsBad(addrmgr.NewKnownAddress(minutesOldNa, 10, minutesOld, monthOld, true, 0)) {
		t.Errorf("test case 9: addresses that have not succeeded in too long are bad.")
	}

	//Test an address that should work. 
	if addrmgr.KnownAddressIsBad(addrmgr.NewKnownAddress(minutesOldNa, 2, minutesOld, hoursOld, true, 0)) {
		t.Errorf("test case 10: This should be a valid address.")
	}
}
