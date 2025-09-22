package kademlia

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
)

// Join: add bootstrap (if any) and warm the table by looking up our own ID.
func (k *Kademlia) Join(bootstrap *Contact) {
	if bootstrap != nil {
		k.routingTable.AddContact(*bootstrap)
		if k.network != nil {
			_ = k.network.SendPingMessage(bootstrap) // nudge mutual visibility
		}
	}
	// Kademlia bootstrap: iterative lookup on our own ID (best-effort)
	if k.network != nil {
		me := &Contact{ID: k.routingTable.me.ID, Address: k.routingTable.me.Address}
		_ = k.iterativeLookupContact(me)
	}
}

// Put: use the network (STORE to K closest); fall back to local when offline.
func (k *Kademlia) Put(data []byte) (string, error) {
	// hash must match network.hashData (SHA-1)
	sum := sha1.Sum(data)
	key := hex.EncodeToString(sum[:])

	if k.network != nil {
		if err := k.network.SendStoreMessage(data); err != nil {
			return "", err
		}
		return key, nil
	}

	// offline fallback
	k.storeMutex.Lock()
	if k.store == nil {
		k.store = make(map[string][]byte)
	}
	k.store[key] = append([]byte(nil), data...) // store a copy
	k.storeMutex.Unlock()
	return key, nil
}

// Get: use the network (FIND_VALUE); fall back to local when offline/not found.
// NOTE: we currently don’t track the “from” node, so we return nil for it.
func (k *Kademlia) Get(key string) ([]byte, *Contact, error) {
	if k.network != nil {
		if data, err := k.network.SendFindDataMessage(key); err == nil && data != nil {
			return data, nil, nil
		}
	}

	// local fallback
	k.storeMutex.RLock()
	val, ok := k.store[key]
	k.storeMutex.RUnlock()
	if ok {
		cp := make([]byte, len(val))
		copy(cp, val)
		return cp, nil, nil
	}
	return nil, nil, errors.New("not found")
}
