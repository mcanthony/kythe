/*
 * Copyright 2015 Google Inc. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package table implements lookup table interfaces for protobufs.
package table

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"reflect"
	"sync"

	"kythe.io/kythe/go/storage/keyvalue"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
)

// Proto is a key-value direct lookup table with protobuf values.
type Proto interface {
	// Lookup unmarshals the value for the given key into msg, returning any
	// error.  If the key was not found, ErrNoSuchKey is returned.
	Lookup(ctx context.Context, key []byte, msg proto.Message) error

	// Put marshals msg and writes it as the value for the given key.
	Put(ctx context.Context, key []byte, msg proto.Message) error

	// Close release the underlying resources for the table.
	Close(context.Context) error
}

// ProtoResult is a result for a single key given to a LookupBatch call.
type ProtoResult struct {
	Key   []byte
	Value proto.Message
	Err   error
}

// ProtoBatch is a key-value lookup table with batch retrieval.
type ProtoBatch interface {
	Proto

	// LookupBatch retrieves the values for the given slice of keys, unmarshaling
	// each value into a new proto.Message of the same type as msg, and sending it
	// as a ProtoResult to the returned channel.
	LookupBatch(ctx context.Context, keys [][]byte, msg proto.Message) (<-chan ProtoResult, error)
}

// ProtoBatchParallel implements the ProtoBatch interface by parallelizing calls
// to Lookup.
type ProtoBatchParallel struct{ Proto }

// LookupBatch implements the ProtoBatch interface.
func (p ProtoBatchParallel) LookupBatch(ctx context.Context, keys [][]byte, msg proto.Message) (<-chan ProtoResult, error) {
	typ := reflect.TypeOf(msg)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	ch := make(chan ProtoResult, len(keys))

	var wg sync.WaitGroup
	wg.Add(len(keys))
	for _, key := range keys {
		go func(key []byte) {
			msg := reflect.New(typ).Interface().(proto.Message)
			err := p.Lookup(ctx, key, msg)
			ch <- ProtoResult{Key: key, Value: msg, Err: err}
			wg.Done()
		}(key)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	return ch, nil
}

// Inverted is an inverted index lookup table for []byte values with associated
// []byte keys.  Keys and values should not contain \000 bytes.
type Inverted interface {
	// Lookup returns a slice of []byte keys associated with the given value.  If
	// prefix is true, all keys associated with values with val as their prefix
	// will be returned, otherwise val is exactly matched.
	Lookup(ctx context.Context, val []byte, prefix bool) ([][]byte, error)

	// Contains determines whether there is an association between key and val.
	// If prefix is true, if key is associated with any value with val as its
	// prefix true will be returned, otherwise val must be exactly associated with
	// key.
	Contains(ctx context.Context, key, val []byte, prefix bool) (bool, error)

	// Put writes an entry associating val with key.
	Put(ctx context.Context, key, val []byte) error

	// Close release the underlying resources for the table.
	Close(context.Context) error
}

// KVProto implements a Proto table using a keyvalue.DB.
type KVProto struct{ keyvalue.DB }

// ErrNoSuchKey is returned when a value was not found for a particular key.
var ErrNoSuchKey = errors.New("no such key")

// Lookup implements part of the Proto interface.
func (t *KVProto) Lookup(_ context.Context, key []byte, msg proto.Message) error {
	iter, err := t.ScanPrefix(key, nil)
	if err != nil {
		return fmt.Errorf("table iterator error: %v", err)
	}
	defer iter.Close()
	k, v, err := iter.Next()
	if err == io.EOF {
		return ErrNoSuchKey
	} else if err != nil {
		return err
	} else if !bytes.Equal(key, k) {
		return ErrNoSuchKey
	}
	if err := proto.Unmarshal(v, msg); err != nil {
		return fmt.Errorf("proto unmarshal error: %v", err)
	}
	return nil
}

// Put implements part of the Proto interface.
func (t *KVProto) Put(_ context.Context, key []byte, msg proto.Message) error {
	rec, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	wr, err := t.Writer()
	if err != nil {
		return err
	}
	defer wr.Close()
	return wr.Write(key, rec)
}

// Close implements part of the Proto interface.
func (t *KVProto) Close(_ context.Context) error { return t.DB.Close() }

// KVInverted implements an Inverted table in a keyvalue.DB.
type KVInverted struct{ keyvalue.DB }

// Lookup implements part of the Inverted interface.
func (i *KVInverted) Lookup(_ context.Context, val []byte, prefixLookup bool) ([][]byte, error) {
	var results [][]byte
	if !prefixLookup {
		val = exactInvertedPrefix(val)
	}
	return results, i.scan(val, func(k []byte) bool {
		i := bytes.IndexByte(k, invertedKeySep)
		if i == -1 {
			log.Printf("WARNING: skipping invalid index key: %q", string(k))
		} else {
			results = append(results, k[i+1:])
		}
		return true
	})
}

// Contains implements part of the Inverted interface.
func (i *KVInverted) Contains(_ context.Context, key, val []byte, prefixLookup bool) (found bool, err error) {
	if prefixLookup {
		err = i.scan(val, func(k []byte) bool {
			i := bytes.IndexByte(k, invertedKeySep)
			if i == -1 {
				log.Printf("WARNING: skipping invalid index key: %q", string(k))
				return true
			}
			found = bytes.Equal(k[i+1:], key)
			return !found
		})
		return
	}

	iKey := invertedKey(key, val)
	err = i.scan(iKey, func(k []byte) bool {
		found = bytes.Equal(k, iKey)
		return false
	})
	return
}

func (i *KVInverted) scan(v []byte, f func(k []byte) bool) error {
	iter, err := i.ScanPrefix(v, nil)
	if err != nil {
		return fmt.Errorf("table iterator error: %v", err)
	}
	defer iter.Close()

	for {
		k, _, err := iter.Next()
		if err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}

		if !bytes.HasPrefix(k, v) {
			return nil
		} else if !f(k) {
			return nil
		}
	}
}

var emptyValue = []byte{}

// Put implements part of the Inverted interface.
func (i *KVInverted) Put(_ context.Context, key, val []byte) error {
	wr, err := i.Writer()
	if err != nil {
		return err
	}
	defer wr.Close()
	return wr.Write(invertedKey(key, val), emptyValue)
}

// Close implements part of the Inverted interface.
func (i *KVInverted) Close(_ context.Context) error { return i.DB.Close() }

const invertedKeySep = '\000'

func invertedKey(key, val []byte) []byte {
	var buf bytes.Buffer
	buf.Grow(len(val) + 1 + len(key))
	buf.Write(val)
	buf.WriteByte(invertedKeySep)
	buf.Write(key)
	return buf.Bytes()
}

func exactInvertedPrefix(val []byte) []byte {
	var buf bytes.Buffer
	buf.Grow(len(val) + 1)
	buf.Write(val)
	buf.WriteByte(invertedKeySep)
	return buf.Bytes()
}
