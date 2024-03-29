// Copyright (c) 2014 The gomqtt Authors. All rights reserved.
//
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

package broker

import (
	"sync"

	"github.com/gomqtt/packet"
	"github.com/satori/go.uuid"
)

/* state */

// a state keeps track of the clients current state
type state struct {
	sync.Mutex

	current byte
}

// create new state
func newState(init byte) *state {
	return &state{
		current: init,
	}
}

// set will change to the specified state
func (s *state) set(state byte) {
	s.Lock()
	defer s.Unlock()

	s.current = state
}

// get will retrieve the current state
func (s *state) get() byte {
	s.Lock()
	defer s.Unlock()

	return s.current
}

/* Context */

// A Context is a store for custom data.
type Context struct {
	store map[string]interface{}
	mutex sync.Mutex
}

// NewContext returns a new Context.
func NewContext() *Context {
	return &Context{
		store: make(map[string]interface{}),
	}
}

// Set sets the passed value for the key in the context.
func (c *Context) Set(key string, value interface{}) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.store[key] = value
}

// Get returns the stored valued for the passed key.
func (c *Context) Get(key string) interface{} {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.store[key]
}

/* fakeClient */

// a fake client for testing backend implementations
type fakeClient struct {
	in  []*packet.Message
	ctx *Context
}

// returns a new fake client
func newFakeClient() *fakeClient {
	ctx := NewContext()
	ctx.Set("uuid", uuid.NewV1().String())

	return &fakeClient{
		ctx: ctx,
	}
}

// publish will append the message to the in slice
func (c *fakeClient) Publish(msg *packet.Message) bool {
	c.in = append(c.in, msg)
	return true
}

// does nothing atm
func (c *fakeClient) Close(clean bool) {}

// returns the context
func (c *fakeClient) Context() *Context {
	return c.ctx
}
