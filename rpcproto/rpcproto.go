// Copyright (c) 2015 Monetas.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcproto

import (
	"encoding/base64"

	"golang.org/x/net/context"
)

//go:generate protoc --go_out=plugins=grpc:. rpc.proto

// BasicAuthCredentials is used for sending credentials to gRPC server using
// Basic authentication method.
type BasicAuthCredentials string

// GetRequestMetadata implements the credentials.Credentials interface.
func (cred BasicAuthCredentials) GetRequestMetadata(ctx context.Context) (map[string]string, error) {
	return map[string]string{
		"authorization": "Basic " + string(cred),
	}, nil
}

// NewBasicAuthCredentials creates a new BasicAuthCredentials instance based on
// the provided username and password.
func NewBasicAuthCredentials(username, password string) BasicAuthCredentials {
	return BasicAuthCredentials(base64.StdEncoding.EncodeToString([]byte(username + ":" + password)))
}
