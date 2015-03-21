package addrmgr

import (
	"time"

	"github.com/monetas/bmd/wire"
)

func KnownAddressIsBad(ka *KnownAddress) bool {
	return ka.isBad()
}

func KnownAddressChance(ka *KnownAddress) float64 {
	return ka.chance()
}

func NewKnownAddress(na *wire.NetAddress, attempts int,
	lastattempt, lastsuccess time.Time, tried bool, refs int) *KnownAddress {
	return &KnownAddress{na:na, attempts:attempts, lastattempt:lastattempt, lastsuccess:lastsuccess, tried:tried, refs:refs}
}
