// Copyright © 2018 Heptio
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package grpc

import (
	"sync/atomic"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

// Resource represents a source of proto.Messages that can be registered
// for interest.
type Resource interface {
	// Contents returns the contents of this resource and resources names.
	Contents() ([]proto.Message, []string)

	// Query returns an entry for each resource name supplied.
	Query(names []string) []proto.Message

	// Register registers ch to receive a value when Notify is called.
	Register(name string, eventCh chan<- PushEvent)
	Unregister(name string)

	// TypeURL returns the typeURL of messages returned from Values.
	TypeURL() string

	Nonce() int
}

// ResourceGetter is a cache of Resource by serviceClusterID and typeURL
type ResourceGetter interface {
	// Get returns Resource by clusterID, appName and typeURL. If resource not found, return ok = false
	Get(appName, typeURL, clusterID string) (result Resource, ok bool)
}

// xdsHandler implements the Envoy xDS gRPC protocol.
type xdsHandler struct {
	logger      logrus.FieldLogger
	connections counter
	resources   ResourceGetter
}

type grpcStream interface {
	Send(*v2.DiscoveryResponse) error
	Recv() (*v2.DiscoveryRequest, error)
	grpc.ServerStream
}

type PushEvent interface {
	ResourceType() string
	Nonce() int
}

// stream processes a stream of DiscoveryRequests.
func (xh *xdsHandler) stream(st grpcStream) error {
	conID := xh.connections.next()
	logger := xh.getLoggerFromStream(st, conID)
	logger.Debug("new connection")
	con := newEnvoyConnecton(logger, xh.resources, st, conID)
	return con.ProcessStream()
}

func (xh *xdsHandler) getLoggerFromStream(stream grpc.Stream, conID uint64) logrus.Ext1FieldLogger {
	// bump connection counter and set it as a field on the logger
	peerInfo, ok := peer.FromContext(stream.Context())
	var conAddr string
	if ok {
		conAddr = peerInfo.Addr.String()
	}
	return xh.logger.WithFields(logrus.Fields{
		"connection": conID,
		"address":    conAddr,
	})
}

// toAny converts the contents of a resourcer's Values to the
// respective slice of *any.Any.
func toAny(typeURL string, values []proto.Message) ([]*any.Any, error) {
	var resources []*any.Any
	for _, value := range values {
		v, err := ptypes.MarshalAny(value)
		if err != nil {
			return nil, err
		}
		resources = append(resources, v)
	}
	return resources, nil
}

// counter holds an atomically incrementing counter.
type counter uint64

func (c *counter) next() uint64 {
	return atomic.AddUint64((*uint64)(c), 1)
}
