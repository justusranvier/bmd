// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bmec

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"io"
	"math/big"
)

// ErrInvalidMAC results when Message Authentication Check (MAC) fails during
// decryption. This happens because of either invalid private key or corrupt
// ciphertext.
var ErrInvalidMAC = errors.New("invalid mac hash")

// ErrInputTooShort results when the input ciphertext to the Decrypt function
// is less than 134 bytes long.
var ErrInputTooShort = errors.New("ciphertext too short")

var ErrUnsupportedCurve = errors.New("unsupported curve")

var ErrInvalidXLength = errors.New("invalid X length, must be 32")

var ErrInvalidYLength = errors.New("invalid Y length, must be 32")

var ErrInvalidPadding = errors.New("invalid PKCS padding")

// Encrypt encrypts data for the target public key using AES-256-CBC. It also
// generates a private key (the pubkey of which is also in the output). The
// 'structure' that it encodes everything into is:
//
//	struct {
//		// Initialization Vector used for AES-256-CBC
//		IV [16]byte
//		// Public Key: curve(2) + len_of_pubkeyX(2) + pubkeyX +
//		// len_of_pubkeyY(2) + pubkeyY (curve = 714 = secp256k1, from OpenSSL)
//		PublicKey [70]byte
//		// Cipher text
//		Data []byte
//		// HMAC-SHA-256 Message Authentication Code
//		HMAC [32]byte
//	}
func Encrypt(pubkey *PublicKey, in []byte) (out []byte, err error) {
	ephemeral, err := NewPrivateKey(S256())
	if err != nil {
		return
	}
	ecdhKey := ephemeral.GenerateSharedSecret(pubkey)
	derivedKey := sha512.Sum512(ecdhKey)
	key_e := derivedKey[:32]
	key_m := derivedKey[32:]

	paddedIn := addPKCSPadding(in)
	out = make([]byte, aes.BlockSize+70+len(paddedIn)+sha256.Size)
	iv := out[:aes.BlockSize]
	if _, err = io.ReadFull(rand.Reader, iv); err != nil {
		return
	}
	// start writing public key
	// curve and X length: 0x02CA = 714, 0x20 = 32
	copy(out[aes.BlockSize:aes.BlockSize+4], []byte{0x02, 0xCA, 0x00, 0x20})
	// X
	copy(out[aes.BlockSize+4:aes.BlockSize+4+32],
		ephemeral.PubKey().X.Bytes())
	// Y length
	copy(out[aes.BlockSize+4+32:aes.BlockSize+4+32+2], []byte{0x00, 0x20})
	// Y
	copy(out[aes.BlockSize+4+32+2:aes.BlockSize+4+32+2+32],
		ephemeral.PubKey().Y.Bytes())

	// start encryption
	block, err := aes.NewCipher(key_e)
	if err != nil {
		return
	}
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(out[aes.BlockSize+70:len(out)-sha256.Size], paddedIn)

	// start HMAC-SHA-256
	hm := hmac.New(sha256.New, key_m)
	hm.Write(out[:len(out)-sha256.Size])          // everything is hashed
	copy(out[len(out)-sha256.Size:], hm.Sum(nil)) // write checksum

	return
}

// Decrypt decrypts data that was encrypted using the Encrypt function.
func Decrypt(priv *PrivateKey, in []byte) (out []byte, err error) {
	if len(in) < aes.BlockSize+70+aes.BlockSize+sha256.Size {
		return nil, ErrInputTooShort
	}

	// read iv
	iv := in[:aes.BlockSize]

	// start reading pubkey
	if binary.BigEndian.Uint16(in[aes.BlockSize:aes.BlockSize+2]) != 714 {
		return nil, ErrUnsupportedCurve
	}
	if binary.BigEndian.Uint16(in[aes.BlockSize+2:aes.BlockSize+4]) != 32 {
		return nil, ErrInvalidXLength
	}
	xBytes := in[aes.BlockSize+4 : aes.BlockSize+4+32]
	if binary.BigEndian.Uint16(in[aes.BlockSize+4+32:aes.BlockSize+4+32+2]) != 32 {
		return nil, ErrInvalidYLength
	}
	yBytes := in[aes.BlockSize+4+32+2 : aes.BlockSize+4+32+2+32]
	pubkey := new(PublicKey)
	pubkey.Curve = priv.ToECDSA().Curve
	pubkey.X = new(big.Int).SetBytes(xBytes)
	pubkey.Y = new(big.Int).SetBytes(yBytes)

	// check for cipher text length
	if (len(in)-aes.BlockSize-70-sha256.Size)%aes.BlockSize != 0 {
		return nil, ErrInvalidPadding // not padded to 16 bytes
	}

	// read hmac
	messageMAC := in[len(in)-sha256.Size:]

	// generate shared secret
	ecdhKey := priv.GenerateSharedSecret(pubkey)
	derivedKey := sha512.Sum512(ecdhKey)
	key_e := derivedKey[:32]
	key_m := derivedKey[32:]

	// verify mac
	hm := hmac.New(sha256.New, key_m)
	hm.Write(in[:len(in)-sha256.Size]) // everything is hashed
	expectedMAC := hm.Sum(nil)
	if !bytes.Equal(messageMAC, expectedMAC) {
		return nil, ErrInvalidMAC
	}

	// start decryption
	block, err := aes.NewCipher(key_e)
	if err != nil {
		return
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	// same length as ciphertext
	plaintext := make([]byte, len(in)-aes.BlockSize-70-sha256.Size)
	mode.CryptBlocks(plaintext, in[aes.BlockSize+70:len(in)-sha256.Size])

	out, err = removePKCSPadding(plaintext)
	return
}

// Implement PKCS#7 padding with block size of 16 (AES block size).

// addPKCSPadding adds padding to a block of data
func addPKCSPadding(src []byte) []byte {
	padding := aes.BlockSize - len(src)%aes.BlockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(src, padtext...)
}

// removePKCSPadding removes padding from data that was added with addPKCSPadding
func removePKCSPadding(src []byte) ([]byte, error) {
	length := len(src)
	padLength := int(src[length-1])
	if padLength > aes.BlockSize || length < aes.BlockSize {
		return nil, ErrInvalidPadding
	}

	return src[:length-padLength], nil
}
