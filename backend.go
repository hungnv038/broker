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
	"github.com/gomqtt/tools"
)

// A Backend provides effective queuing functionality to a Broker and its Clients.
type Backend interface {
	// Authenticate should authenticate the client using the user and password
	// values and return true if the client is eligible to continue or false
	// when the broker should terminate the connection.
	Authenticate(client Client, user, password string) (bool, error)

	// Setup is called when a new client comes online and is successfully
	// authenticated. Setup should return the already stored session for the
	// supplied id or create and return a new one. If clean is set to true it
	// should additionally reset the session. If the supplied id has a zero
	// length, a new session is returned that is not stored further.
	//
	// Optional: The backend may close any existing clients that use the same
	// client id. It may also start a background process that forwards any missed
	// messages that match the clients offline subscriptions.
	//
	// Note: In this call the Backend may also allocate other resources and
	// setup the client for further usage as the broker will acknowledge the
	// connection when the call returns.
	Setup(client Client, id string, clean bool) (Session, bool, error)

	// Subscribe should subscribe the passed client to the specified topic and
	// call Publish with any incoming messages. It should also return the stored
	// retained messages that match the specified topic.
	//
	// Optional: Subscribe may also return a concatenated list of retained messages
	// and missed offline messages, if the later has not been handled already in
	// the Setup call.
	Subscribe(client Client, topic string) ([]*packet.Message, error)

	// Unsubscribe should unsubscribe the passed client from the specified topic.
	Unsubscribe(client Client, topic string) error

	// Publish should forward the passed message to all other clients that hold
	// a subscription that matches the messages topic. It should also store the
	// message if Retain is set to true and the payload does not have a zero
	// length. If the payload has a zero length and Retain is set to true the
	// currently retained message for that topic should be removed.
	Publish(client Client, msg *packet.Message) error

	// Terminate is called when the client goes offline. Terminate should
	// unsubscribe the passed client from all previously subscribed topics.
	//
	// Optional: The backend may convert a clients subscriptions into offline
	// subscriptions, which allows missed messages to be forwarded on the next
	// connect.
	//
	// Note: The Backend may also cleanup previously allocated resources for
	// that client as the broker will close the connection when the call
	// returns.
	Terminate(client Client) error
}

// A MemoryBackend stores everything in memory.
type MemoryBackend struct {
	Logins map[string]string

	queue         *tools.Tree
	retained      *tools.Tree
	offlineQueue  *tools.Tree

	sessions      map[string]*MemorySession
	sessionsMutex sync.Mutex
}

// NewMemoryBackend returns a new MemoryBackend.
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		queue:        tools.NewTree(),
		retained:     tools.NewTree(),
		offlineQueue: tools.NewTree(),
		sessions:     make(map[string]*MemorySession),
	}
}

// Authenticate authenticates a clients credentials by matching them to the
// saved Logins map.
func (m *MemoryBackend) Authenticate(client Client, user, password string) (bool, error) {
	// allow all if there are no logins
	if m.Logins == nil {
		return true, nil
	}

	// check login
	if pw, ok := m.Logins[user]; ok && pw == password {
		return true, nil
	}

	return false, nil
}

// Setup returns the already stored session for the supplied id or creates
// and returns a new one. If clean is set to true it will additionally reset
// the session. If the supplied id has a zero length, a new session is returned
// that is not stored further. If an existing session has been found it will
// retrieve all stored messages from offline subscriptions and begin with
// forwarding them in a separate goroutine. Furthermore, it will disconnect
// any client connected with the same client id.
func (m *MemoryBackend) Setup(client Client, id string, clean bool) (Session, bool, error) {
	m.sessionsMutex.Lock()
	defer m.sessionsMutex.Unlock()

	// save clean flag
	client.Context().Set("clean", clean)

	// return a new temporary session if id is zero
	if len(id) == 0 {
		sess := NewMemorySession()
		client.Context().Set("session", sess)
		return sess, false, nil
	}

	// retrieve stored session
	sess, ok := m.sessions[id]

	// when found
	if ok {
		// check if session already has a client
		if sess.currentClient != nil {
			sess.currentClient.Close(true)
		}

		// set current client
		sess.currentClient = client

		// reset session if clean is true
		if clean {
			sess.Reset()
		}

		// remove all session from the offline queue
		m.offlineQueue.Clear(sess)

		// send all missed messages in another goroutine
		go func() {
			for _, msg := range sess.missed() {
				client.Publish(msg)
			}
		}()

		// returned stored session
		client.Context().Set("session", sess)
		return sess, true, nil
	}

	// create fresh session
	sess = NewMemorySession()
	sess.currentClient = client

	// save session
	m.sessions[id] = sess

	// return new stored session
	client.Context().Set("session", sess)
	return sess, false, nil
}

// Subscribe will subscribe the passed client to the specified topic and
// begin to forward messages by calling the clients Publish method.
// It will also return the stored retained messages matching the supplied
// topic.
func (m *MemoryBackend) Subscribe(client Client, topic string) ([]*packet.Message, error) {
	// add client to queue
	m.queue.Add(topic, client)

	// get retained messages
	values := m.retained.Search(topic)
	var msgs []*packet.Message

	// convert types
	for _, value := range values {
		if msg, ok := value.(*packet.Message); ok {
			msgs = append(msgs, msg)
		}
	}

	return msgs, nil
}

// Unsubscribe will unsubscribe the passed client from the specified topic.
func (m *MemoryBackend) Unsubscribe(client Client, topic string) error {
	// remove client from queue
	m.queue.Remove(topic, client)

	return nil
}

// Publish will forward the passed message to all other subscribed clients.
// It will also store the message if Retain is set to true. If the supplied
// message has additionally a zero length payload, the backend removes the
// currently retained message. Finally, it will also add the message to all
// sessions that have an offline subscription.
func (m *MemoryBackend) Publish(client Client, msg *packet.Message) error {
	// check retain flag
	if msg.Retain {
		if len(msg.Payload) > 0 {
			m.retained.Set(msg.Topic, msg)
		} else {
			m.retained.Empty(msg.Topic)
		}
	}

	// publish directly to clients
	for _, v := range m.queue.Match(msg.Topic) {
		if client, ok := v.(Client); ok {
			client.Publish(msg)
		}
	}

	// queue for offline clients
	for _, v := range m.offlineQueue.Match(msg.Topic) {
		if session, ok := v.(*MemorySession); ok {
			session.queue(msg)
		}
	}

	return nil
}

// Terminate will unsubscribe the passed client from all previously subscribed
// topics. If the client connect with clean=true it will also clean the session.
// Otherwise it will create offline subscriptions for all QOS 1 and QOS 2
// subscriptions.
func (m *MemoryBackend) Terminate(client Client) error {
	m.sessionsMutex.Lock()
	defer m.sessionsMutex.Unlock()

	// remove client from queue
	m.queue.Clear(client)

	// get session
	session, ok := client.Context().Get("session").(*MemorySession)
	if ok {
		// reset stored client
		session.currentClient = nil

		// check if the client connected with clean=true
		clean, ok := client.Context().Get("clean").(bool)
		if ok && clean {
			// reset session
			session.Reset()
			return nil
		}

		// otherwise get stored subscriptions
		subscriptions, err := session.AllSubscriptions()
		if err != nil {
			return err
		}

		// iterate through stored subscriptions
		for _, sub := range subscriptions {
			if sub.QOS >= 1 {
				// session to offline queue
				m.offlineQueue.Add(sub.Topic, session)
			}
		}
	}

	return nil
}
