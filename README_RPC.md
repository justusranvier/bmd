# JSON-RPC over WebSockets

The RPC server listens for JSON-RPC requests over a WebSockets connection at '/'.
This is because the connection is supposed to be bidirectional, to make it
possible for the server to push data to connected clients.

## Points to Note
- All binary data is base64 encoded.
- Unauthenticated client connection is closed after `rpcAuthTimeoutSeconds`
seconds.


## RPC Client API

Connected clients are expected to implement the following API. This ensures that
the server can push data to where it's needed. If the server is unable to call
a remote method, the connection is immediately terminated.

```go
func ReceiveMessage(object []byte, counter uint64)
```
```go
func ReceiveBroadcast(object []byte, counter uint64)
```
```go
func ReceiveGetpubkey(object []byte, counter uint64)
```
```go
func ReceivePubkey(object []byte, counter uint64)
```
```go
func ReceiveUnknownObject(object []byte, counter uint64)
```
Receive objects of the given type from the server. Order of receiving objects
(especially the initial unsynchronized ones) may be random. Client
implementations must not rely on sequential values of counter. However, values
of counter are guaranteed to be unique.

## RPC Calls
-----------

```go
func Authenticate(username string, password string) bool
```
Authenticate the current connection with the specified credentials. The boolean
return value tells whether the attempt succeeded or failed.

```go
func SendObject(object []byte) uint64
```
Send the specified object onto the network. object is a byte slice containing
a serialized object (just the payload part of an object message). The object is
first verified, then inserted into bmd's database and advertised onto the
network. Return value is the counter value of the inserted object.

```go
type Identity struct { // basically a dictionary
	address             string
	nonceTrialsPerByte  uint64
	extraBytes          uint64
	signingKey          []byte
	encryptionKey       []byte
}

func GetIdentity(address string) Identity
```
Retrieve the public identity of the given address. This is constructed from
public keys stored in the database. If the public key for the specified address
doesn't exist, an error is returned.

```go
func SubscribeMessages(fromCounter uint64)
```
```go
func SubscribeBroadcasts(fromCounter uint64)
```
```go
func SubscribeGetpubkeys(fromCounter uint64)
```
```go
func SubscribePubkeys(fromCounter uint64)
```
```go
func SubscribeUnknownObjects(fromCounter uint64)
```

Subscribe the client to the given object messages that have counter values
starting from `fromCounter`. These objects are pushed to the client side using
RPC Client API (refer above).