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
// Author: Vivek Menezes (vivek.menezes@gmail.com)

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/cockroach/client"
	"github.com/cockroachdb/cockroach/security"
	"github.com/cockroachdb/cockroach/security/securitytest"
	"github.com/cockroachdb/cockroach/server"
	"github.com/cockroachdb/cockroach/util/log"
)

var useTransaction = flag.Bool("use-transaction", true, "Turn off to disable transaction.")

// Makes an id string from an id int.
func makeAccountID(id int) []byte {
	return []byte(fmt.Sprintf("%09d", id))
}

// Bank stores all the bank related state.
type Bank struct {
	db           *client.DB
	numAccounts  int
	numTransfers int32
}

type Account struct {
	Balance int64
}

func (a Account) encode() ([]byte, error) {
	return json.Marshal(a)
}

func (a *Account) decode(b []byte) error {
	return json.Unmarshal(b, a)
}

// Read the balances in all the accounts and return them.
func (bank *Bank) sumAllAccounts() int64 {
	var result int64
	err := bank.db.Tx(func(tx *client.Tx) error {
		scan := tx.Scan(makeAccountID(0), makeAccountID(bank.numAccounts), int64(bank.numAccounts))
		if scan.Err != nil {
			log.Fatal(scan.Err)
		}
		if len(scan.Rows) != bank.numAccounts {
			log.Fatalf("Could only read %d of %d rows of the database.\n", len(scan.Rows), bank.numAccounts)
		}
		// Copy responses into balances.
		for i := 0; i < bank.numAccounts; i++ {
			account := &Account{}
			err := account.decode(scan.Rows[i].ValueBytes())
			if err != nil {
				log.Fatal(err)
			}
			// fmt.Printf("Account %d contains %d$\n", i, account.Balance)
			result += account.Balance
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	return result
}

// continuouslyTransferMoney() keeps moving random amounts between
// random accounts.
func (bank *Bank) continuousMoneyTransfer() {
	for {
		from := makeAccountID(rand.Intn(bank.numAccounts))
		to := makeAccountID(rand.Intn(bank.numAccounts))
		// Continue when from == to
		if bytes.Equal(from, to) {
			continue
		}
		exchangeAmount := rand.Int63n(100)
		// transferMoney transfers exchangeAmount between the two accounts
		transferMoney := func(runner client.Runner) error {
			batchRead := &client.Batch{}
			batchRead.Get(from, to)
			if err := runner.Run(batchRead); err != nil {
				return err
			}
			if batchRead.Results[0].Err != nil {
				return batchRead.Results[0].Err
			}
			// Read from value.
			fromAccount := &Account{}
			err := fromAccount.decode(batchRead.Results[0].Rows[0].ValueBytes())
			if err != nil {
				return err
			}
			// Ensure there is enough cash.
			if fromAccount.Balance < exchangeAmount {
				return nil
			}
			// Read to value.
			toAccount := &Account{}
			errRead := toAccount.decode(batchRead.Results[0].Rows[1].ValueBytes())
			if errRead != nil {
				return errRead
			}
			// Update both accounts.
			batchWrite := &client.Batch{}
			fromAccount.Balance -= exchangeAmount
			toAccount.Balance += exchangeAmount
			if fromValue, err := fromAccount.encode(); err != nil {
				return err
			} else if toValue, err := toAccount.encode(); err != nil {
				return err
			} else {
				batchWrite.Put(fromValue, toValue)
			}
			return runner.Run(batchWrite)
		}
		if *useTransaction {
			if err := bank.db.Tx(func(tx *client.Tx) error { return transferMoney(tx) }); err != nil {
				log.Fatal(err)
			}
		} else if err := transferMoney(bank.db); err != nil {
			log.Fatal(err)
		}
		atomic.AddInt32(&bank.numTransfers, 1)
	}
}

// Initialize all the bank accounts with cash.
func (bank *Bank) initBankAccounts(cash int64) {
	batch := &client.Batch{}
	account := Account{Balance: cash}
	value, err := account.encode()
	if err != nil {
		log.Fatal(err)
	}
	for i := 0; i < bank.numAccounts; i++ {
		batch = batch.Put(makeAccountID(i), value)
	}
	if err := bank.db.Run(batch); err != nil {
		log.Fatal(err)
	}
	log.Info("Done initializing all accounts\n")
}

func (bank *Bank) periodicallyCheckBalances(initCash int64) {
	for {
		// Sleep for a bit to allow money transfers to happen in the background.
		time.Sleep(time.Second)
		fmt.Printf("%d transfers were executed.\n\n", bank.numTransfers)
		// Check that all the money is accounted for.
		totalAmount := bank.sumAllAccounts()
		if totalAmount != int64(bank.numAccounts)*initCash {
			err := fmt.Sprintf("\nTotal cash in the bank = %d.\n", totalAmount)
			log.Fatal(err)
		}
		fmt.Printf("\nThe bank is in good order\n\n")
	}
}

func main() {
	fmt.Printf("A simple program that keeps moving money between bank accounts.\n\n")
	flag.Parse()
	if !*useTransaction {
		fmt.Printf("Use of a transaction has been disabled.\n")
	}
	// Run a test cockroach instance to represent the bank.
	security.SetReadFileFn(securitytest.Asset)
	serv := server.StartTestServer(nil)
	defer serv.Stop()
	// Initialize the bank.
	var bank Bank
	bank.numAccounts = 10
	// Create a database handle
	db, err := client.Open("https://root@" + serv.ServingAddr() + "?certs=test_certs")
	if err != nil {
		log.Fatal(err)
	}
	bank.db = db
	// Initialize all the bank accounts.
	const initCash = 1000
	bank.initBankAccounts(initCash)

	// Start all the money transfer routines.
	const numTransferRoutines = 1000
	for i := 0; i < numTransferRoutines; i++ {
		go bank.continuousMoneyTransfer()
	}

	bank.periodicallyCheckBalances(initCash)
}
