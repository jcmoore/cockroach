// Copyright 2014 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License. See the AUTHORS file
// for names of contributors.
//
// Author: Tobias Schottdorf (tobias.schottdorf@gmail.com)

package multiraft

import (
	"net/rpc"
	"sync"

	"github.com/cockroachdb/cockroach/util"
)

type localInterceptableTransport struct {
	mu        sync.Mutex
	listeners map[NodeID]ServerInterface
	messages  chan *RaftMessageRequest
	Events    chan *interceptMessage
}

// NewLocalInterceptableTransport creates a Transport for local testing use.
// MultiRaft instances sharing the same instance of this Transport can find and
// communicate with each other by ID. Messages are transmitted in the order in
// which they are queued, intercepted and blocked until acknowledged.
func NewLocalInterceptableTransport() Transport {
	lt := &localInterceptableTransport{
		listeners: make(map[NodeID]ServerInterface),
		messages:  make(chan *RaftMessageRequest),
		Events:    make(chan *interceptMessage),
	}
	go lt.start()
	return lt
}

func (lt *localInterceptableTransport) start() {
	for msg := range lt.messages {
		ack := make(chan struct{})
		iMsg := &interceptMessage{
			args: msg,
			ack:  ack,
		}
		lt.Events <- iMsg
		<-ack
		lt.mu.Lock()
		srv, ok := lt.listeners[NodeID(msg.Message.To)]
		lt.mu.Unlock()
		if !ok {
			continue
		}
		srv.RaftMessage(msg, nil)
	}
}

func (lt *localInterceptableTransport) Listen(id NodeID, server ServerInterface) error {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	lt.listeners[id] = server
	return nil
}

func (lt *localInterceptableTransport) Stop(id NodeID) {
	lt.mu.Lock()
	delete(lt.listeners, id)
	lt.mu.Unlock()
}

func (lt *localInterceptableTransport) Connect(id NodeID) (ClientInterface, error) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	_, ok := lt.listeners[id]
	if !ok {
		return nil, util.Errorf("unknown peer: %d", id)
	}
	return lt, nil
}

// an interceptMessage is sent by an interceptableClient when a message is to
// be sent.
type interceptMessage struct {
	args interface{}
	ack  chan<- struct{}
}

// Go implements ClientInterface.
func (lt *localInterceptableTransport) Go(serviceMethod string, args interface{}, reply interface{}, done chan *rpc.Call) *rpc.Call {
	lt.messages <- args.(*RaftMessageRequest)
	if done != nil {
		close(done)
	}
	return nil
}

func (lt *localInterceptableTransport) Close() error {
	return nil
}