bmec
=====

Package bmec implements elliptic curve cryptography needed for working with
Bitmessage (secp256k1 only for now). It is designed so that it may be used with
the standard crypto/ecdsa packages provided with go. A comprehensive suite of
tests is provided to ensure proper functionality. Package bmec is based off the
package btcec, developed by Conformal. The only changes made were the addition
of encryption and decryption to make it compatible with Pyelliptic and useable
with bmd.

Although this package was primarily written for bmd, it has intentionally been
designed so it can be used as a standalone package for any projects needing to
use secp256k1 elliptic curve cryptography.

## Documentation

[![GoDoc](https://godoc.org/github.com/monetas/bmd/bmec?status.png)]
(http://godoc.org/github.com/monetas/bmd/bmec)

Full `go doc` style documentation for the project can be viewed online without
installing this package by using the GoDoc site
[here](http://godoc.org/github.com/monetas/bmd/bmec).

You can also view the documentation locally once the package is installed with
the `godoc` tool by running `godoc -http=":6060"` and pointing your browser to
http://localhost:6060/pkg/github.com/monetas/bmd/bmec

## Installation

```bash
$ go get github.com/monetas/bmd/bmec
```

## Examples

* [Sign Message]
  (http://godoc.org/github.com/monetas/bmd/bmec#example-package--SignMessage)  
  Demonstrates signing a message with a secp256k1 private key that is first
  parsed form raw bytes and serializing the generated signature.

* [Verify Signature]
  (http://godoc.org/github.com/monetas/bmd/bmec#example-package--VerifySignature)  
  Demonstrates verifying a secp256k1 signature against a public key that is
  first parsed from raw bytes. The signature is also parsed from raw bytes.

* [Encryption]
  (http://godoc.org/github.com/monetas/bmd/bmec#example-package--EncryptMessage)
  Demonstrates encrypting a message for a public key that is first parsed from
  raw bytes, then decrypting it using the corresponding private key.

* [Decryption]
  (http://godoc.org/github.com/monetas/bmd/bmec#example-package--DecryptMessage)
  Demonstrates decrypting a message using a private key that is first parsed
  from raw bytes.
