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
	"time"

	"github.com/gomqtt/transport"
)

// The Logger callback handles incoming log messages.
type Logger func(msg string)

// The Broker handles incoming connections and connects them to the backend.
type Broker struct {
	Backend Backend
	Logger  Logger

	ConnectTimeout time.Duration
}

// New returns a new Broker with a basic MemoryBackend.
func New() *Broker {
	return &Broker{
		Backend:        NewMemoryBackend(),
		ConnectTimeout: 10 * time.Second,
	}
}

// Handle takes over responsibility and handles a transport.Conn.
func (b *Broker) Handle(conn transport.Conn) {
	newRemoteClient(b, conn)
}
