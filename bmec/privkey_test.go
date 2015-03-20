package bmec_test

import (
	"bytes"
	"testing"

	"github.com/monetas/bmd/bmec"
)

func TestPrivKeys(t *testing.T) {
	tests := []struct {
		name string
		key  []byte
	}{
		{
			name: "check curve",
			key: []byte{
				0xea, 0xf0, 0x2c, 0xa3, 0x48, 0xc5, 0x24, 0xe6,
				0x39, 0x26, 0x55, 0xba, 0x4d, 0x29, 0x60, 0x3c,
				0xd1, 0xa7, 0x34, 0x7d, 0x9d, 0x65, 0xcf, 0xe9,
				0x3c, 0xe1, 0xeb, 0xff, 0xdc, 0xa2, 0x26, 0x94,
			},
		},
	}

	for _, test := range tests {
		priv, pub := bmec.PrivKeyFromBytes(bmec.S256(), test.key)

		_, err := bmec.ParsePubKey(
			pub.SerializeUncompressed(), bmec.S256())
		if err != nil {
			t.Errorf("%s privkey: %v", test.name, err)
			continue
		}

		hash := []byte{0x0, 0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9}
		sig, err := priv.Sign(hash)
		if err != nil {
			t.Errorf("%s could not sign: %v", test.name, err)
			continue
		}

		if !sig.Verify(hash, pub) {
			t.Errorf("%s could not verify: %v", test.name, err)
			continue
		}

		serializedKey := priv.Serialize()
		if !bytes.Equal(serializedKey, test.key) {
			t.Errorf("%s unexpected serialized bytes - got: %x, "+
				"want: %x", test.name, serializedKey, test.key)
		}
	}
}

func TestGenerateSharedSecret(t *testing.T) {
	privKey1, err := bmec.NewPrivateKey(bmec.S256())
	if err != nil {
		t.Errorf("private key generation error: %s", err)
		return
	}
	privKey2, err := bmec.NewPrivateKey(bmec.S256())
	if err != nil {
		t.Errorf("private key generation error: %s", err)
		return
	}

	secret1 := privKey1.GenerateSharedSecret(privKey2.PubKey())
	secret2 := privKey2.GenerateSharedSecret(privKey1.PubKey())

	if !bytes.Equal(secret1, secret2) {
		t.Errorf("ECDH failed, secrets mismatch - first: %x, second: %x",
			secret1, secret2)
	}
}
