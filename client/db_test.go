// Copyright 2015 The Cockroach Authors.
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
// Author: Peter Mattis (peter.mattis@gmail.com)

package client_test

import (
	"fmt"
	"log"
	"testing"

	"github.com/cockroachdb/cockroach/client"
	"github.com/cockroachdb/cockroach/server"
)

func setup() (*server.TestServer, *client.DB) {
	s := server.StartTestServer(nil)
	db, err := client.Open("https://root@" + s.ServingAddr() + "?certs=test_certs")
	if err != nil {
		log.Fatal(err)
	}
	return s, db
}

func ExampleDB_Get() {
	s, db := setup()
	defer s.Stop()

	result, err := db.Get("aa")
	if err != nil {
		panic(err)
	}
	fmt.Printf("aa=%s\n", result.Rows[0].ValueBytes())

	// Output:
	// aa=
}

func ExampleDB_Put() {
	s, db := setup()
	defer s.Stop()

	if _, err := db.Put("aa", "1"); err != nil {
		panic(err)
	}
	result, err := db.Get("aa")
	if err != nil {
		panic(err)
	}
	fmt.Printf("aa=%s\n", result.Rows[0].ValueBytes())

	// Output:
	// aa=1
}

func ExampleDB_CPut() {
	s, db := setup()
	defer s.Stop()

	if _, err := db.Put("aa", "1"); err != nil {
		panic(err)
	}
	if _, err := db.CPut("aa", "2", "1"); err != nil {
		panic(err)
	}
	result, err := db.Get("aa")
	if err != nil {
		panic(err)
	}
	fmt.Printf("aa=%s\n", result.Rows[0].ValueBytes())

	if _, err = db.CPut("aa", "3", "1"); err == nil {
		panic("expected error from conditional put")
	}
	result, err = db.Get("aa")
	if err != nil {
		panic(err)
	}
	fmt.Printf("aa=%s\n", result.Rows[0].ValueBytes())

	if _, err = db.CPut("bb", "4", "1"); err == nil {
		panic("expected error from conditional put")
	}
	result, err = db.Get("bb")
	if err != nil {
		panic(err)
	}
	fmt.Printf("bb=%s\n", result.Rows[0].ValueBytes())
	if _, err = db.CPut("bb", "4", nil); err != nil {
		panic(err)
	}
	result, err = db.Get("bb")
	if err != nil {
		panic(err)
	}
	fmt.Printf("bb=%s\n", result.Rows[0].ValueBytes())

	// Output:
	// aa=2
	// aa=2
	// bb=
	// bb=4
}

func ExampleDB_Inc() {
	s, db := setup()
	defer s.Stop()

	if _, err := db.Inc("aa", 100); err != nil {
		panic(err)
	}
	result, err := db.Get("aa")
	if err != nil {
		panic(err)
	}
	fmt.Printf("aa=%d\n", result.Rows[0].ValueInt())

	// Output:
	// aa=100
}

func ExampleBatch() {
	s, db := setup()
	defer s.Stop()

	b := db.B.Get("aa").Put("bb", "2")
	if err := db.Run(b); err != nil {
		panic(err)
	}
	for _, result := range b.Results {
		for _, row := range result.Rows {
			fmt.Printf("%s=%s\n", row.Key, row.ValueBytes())
		}
	}

	// Output:
	// aa=
	// bb=2
}

func ExampleDB_Scan() {
	s, db := setup()
	defer s.Stop()

	b := db.B.Put("aa", "1").Put("ab", "2").Put("bb", "3")
	if err := db.Run(b); err != nil {
		panic(err)
	}
	result, err := db.Scan("a", "b", 100)
	if err != nil {
		panic(err)
	}
	for i, row := range result.Rows {
		fmt.Printf("%d: %s=%s\n", i, row.Key, row.ValueBytes())
	}

	// Output:
	// 0: aa=1
	// 1: ab=2
}

func ExampleDB_Del() {
	s, db := setup()
	defer s.Stop()

	if err := db.Run(db.B.Put("aa", "1").Put("ab", "2").Put("ac", "3")); err != nil {
		panic(err)
	}
	if _, err := db.Del("ab"); err != nil {
		panic(err)
	}
	result, err := db.Scan("a", "b", 100)
	if err != nil {
		panic(err)
	}
	for i, row := range result.Rows {
		fmt.Printf("%d: %s=%s\n", i, row.Key, row.ValueBytes())
	}

	// Output:
	// 0: aa=1
	// 1: ac=3
}

func ExampleTx_Commit() {
	s, db := setup()
	defer s.Stop()

	err := db.Tx(func(tx *client.Tx) error {
		return tx.Commit(tx.B.Put("aa", "1").Put("ab", "2"))
	})
	if err != nil {
		panic(err)
	}

	result, err := db.Get("aa", "ab")
	if err != nil {
		panic(err)
	}
	for i, row := range result.Rows {
		fmt.Printf("%d: %s=%s\n", i, row.Key, row.ValueBytes())
	}

	// Output:
	// 0: aa=1
	// 1: ab=2
}

func ExampleDB_Insecure() {
	s := &server.TestServer{}
	s.Ctx = server.NewTestContext()
	s.Ctx.Insecure = true
	if err := s.Start(); err != nil {
		log.Fatalf("Could not start server: %v", err)
	}
	log.Printf("Test server listening on %s: %s", s.Ctx.RequestScheme(), s.ServingAddr())
	defer s.Stop()

	db, err := client.Open("http://root@" + s.ServingAddr())
	if err != nil {
		log.Fatal(err)
	}

	if _, err := db.Put("aa", "1"); err != nil {
		panic(err)
	}
	result, err := db.Get("aa")
	if err != nil {
		panic(err)
	}
	fmt.Printf("aa=%s\n", result.Rows[0].ValueBytes())

	// Output:
	// aa=1
}

func TestOpenArgs(t *testing.T) {
	s := server.StartTestServer(nil)
	defer s.Stop()

	testCases := []struct {
		addr      string
		expectErr bool
	}{
		{"https://root@" + s.ServingAddr() + "?certs=test_certs", false},
		{"https://" + s.ServingAddr() + "?certs=test_certs", false},
		{"https://" + s.ServingAddr() + "?certs=foo", true},
	}

	for _, test := range testCases {
		_, err := client.Open(test.addr)
		if test.expectErr && err == nil {
			t.Errorf("Open(%q): expected an error; got %v", test.addr, err)
		} else if !test.expectErr && err != nil {
			t.Errorf("Open(%q): expected no errors; got %v", test.addr, err)
		}
	}
}
