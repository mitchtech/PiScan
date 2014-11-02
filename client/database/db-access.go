// Copyright Banrai LLC. All rights reserved. Use of this source code is
// governed by the license that can be found in the LICENSE file.

// Package database provides access to the sqlite database on the Pi client

package database

import (
	"github.com/mxk/go-sqlite/sqlite3"
	"io/ioutil"
	"path"
	"strings"
)

const (
	// Default database filename
	SQLITE_FILE = "PiScanDB.sqlite"

	// Default sql definitions file
	TABLE_SQL_DEFINITIONS = "tables.sql"

	// Anonymous Account
	ANONYMOUS_EMAIL    = "anonymous@example.org"
	ANONYMOUS_API_CODE = "12345678-abcd-9ef0-1234-567890abcdef"

	// Prepared Statements
	// User accounts
	ADD_ACCOUNT    = "insert into account (email, api_code) values ($e, $a)"
	GET_ACCOUNT    = "select id, api_code from account where email = $e"
	GET_ACCOUNTS   = "select id, email, api_code from account"
	UPDATE_ACCOUNT = "update account set email = $e, api_code = $a where id = $i"

	// Products
	ADD_ITEM           = "insert into product (barcode, product_desc, product_ind, posted, account) values ($b, $d, $i, strftime('%s','now'), $a)"
	GET_ITEMS          = "select id, barcode, product_desc, product_ind, datetime(posted) from product where account = $a"
	GET_FAVORITE_ITEMS = "select id, barcode, product_desc, product_ind, datetime(posted) from product where is_favorite = 1 and account = $a"
	DELETE_ITEM        = "delete from product where id = $i"
	FAVORITE_ITEM      = "update product set is_favorite = 1 where id = $i"
	UNFAVORITE_ITEM    = "update product set is_favorite = 0 where id = $i"
)

type Item struct {
	Id      int64
	Desc    string
	Barcode string
	Index   int64
	Since   string
}

func (i *Item) Add(db *sqlite3.Conn, a *Account) error {
	// insert the Item object
	args := sqlite3.NamedArgs{"$b": i.Barcode,
		"$d": i.Desc,
		"$i": i.Index,
		"$a": a.Id}
	return db.Exec(ADD_ITEM, args)
}

func (i *Item) Delete(db *sqlite3.Conn) error {
	// delete the Item
	args := sqlite3.NamedArgs{"$i": i.Id}
	return db.Exec(DELETE_ITEM, args)
}

func (i *Item) Favorite(db *sqlite3.Conn) error {
	// update the Item, to show it is a favorite for this Account
	args := sqlite3.NamedArgs{"$i": i.Id}
	return db.Exec(FAVORITE_ITEM, args)
}

func (i *Item) Unfavorite(db *sqlite3.Conn) error {
	// update the Item, to show it is not a favorite for this Account
	args := sqlite3.NamedArgs{"$i": i.Id}
	return db.Exec(UNFAVORITE_ITEM, args)
}

func GetItems(db *sqlite3.Conn, a *Account) ([]*Item, error) {
	// find all the items for this account
	results := make([]*Item, 0)

	args := sqlite3.NamedArgs{"$a": a.Id}
	row := make(sqlite3.RowMap)
	for s, err := db.Query(GET_ITEMS, args); err == nil; err = s.Next() {
		var rowid int64
		s.Scan(&rowid, row)

		barcode, barcodeFound := row["barcode"]
		desc, descFound := row["product_desc"]
		ind, indFound := row["product_ind"]
		since, sinceFound := row["posted"]
		if barcodeFound {
			result := new(Item)
			result.Id = rowid
			result.Barcode = barcode.(string)
			if descFound {
				result.Desc = desc.(string)
			}
			if indFound {
				result.Index = ind.(int64)
			}
			if sinceFound {
				result.Since = since.(string)
			}
			results = append(results, result)
		}
	}

	return results, nil
}

type Account struct {
	Id      int64
	Email   string
	APICode string
}

func (a *Account) Add(db *sqlite3.Conn) error {
	// insert the Account object
	args := sqlite3.NamedArgs{"$e": a.Email, "$a": a.APICode}
	return db.Exec(ADD_ACCOUNT, args)
}

func (a *Account) Update(db *sqlite3.Conn, newEmail, newApi string) error {
	// update this Account's email and API code
	args := sqlite3.NamedArgs{"$i": a.Id, "$e": newEmail, "$a": newApi}
	return db.Exec(UPDATE_ACCOUNT, args)
}

func GetAccount(db *sqlite3.Conn, email string) (*Account, error) {
	// get the account corresponding to this email
	result := new(Account)

	args := sqlite3.NamedArgs{"$e": email}
	row := make(sqlite3.RowMap)
	for s, err := db.Query(GET_ACCOUNT, args); err == nil; err = s.Next() {
		var rowid int64
		s.Scan(&rowid, row)

		api, apiFound := row["api_code"]
		if apiFound {
			result.APICode = api.(string)
			result.Id = rowid
			result.Email = email
			break
		}
	}

	return result, nil
}

func GetAllAccounts(db *sqlite3.Conn) ([]*Account, error) {
	// find all the accounts currently registered
	results := make([]*Account, 0)

	row := make(sqlite3.RowMap)
	for s, err := db.Query(GET_ACCOUNTS); err == nil; err = s.Next() {
		var rowid int64
		s.Scan(&rowid, row)

		email, emailFound := row["email"]
		api, apiFound := row["api_code"]
		if emailFound && apiFound {
			result := new(Account)
			result.APICode = api.(string)
			result.Id = rowid
			result.Email = email.(string)
			results = append(results, result)
		}
	}

	return results, nil
}

func FetchAnonymousAccount(db *sqlite3.Conn) (*Account, error) {
	// return the existing Anonymous account
	anon, anonErr := GetAccount(db, ANONYMOUS_EMAIL)

	// or create it, if it does not exist yet
	if anon.Email == "" && anon.APICode == "" {
		anon = new(Account)
		anon.Email = ANONYMOUS_EMAIL
		anon.APICode = ANONYMOUS_API_CODE
		anonErr = anon.Add(db)
		if anonErr == nil {
			// make sure the Id value is correct
			return GetAccount(db, ANONYMOUS_EMAIL)
		}
	}

	return anon, anonErr
}

type ConnCoordinates struct {
	DBPath       string
	DBFile       string
	DBTablesPath string
}

func InitializeDB(coords ConnCoordinates) (*sqlite3.Conn, error) {
	// attempt to open the sqlite db file
	db, dbErr := sqlite3.Open(path.Join(coords.DBPath, coords.DBFile))
	if dbErr != nil {
		return db, dbErr
	}

	// load the table definitions file
	content, err := ioutil.ReadFile(path.Join(coords.DBTablesPath, TABLE_SQL_DEFINITIONS))
	if err != nil {
		return db, err
	}

	// attempt to create (if not exists) each table
	tables := strings.Split(string(content), ";")
	for _, table := range tables {
		err = db.Exec(table)
		if err != nil {
			return db, err
		}
	}

	return db, nil
}