package models

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/quan-to/remote-signer/openpgp"
	r "gopkg.in/rethinkdb/rethinkdb-go.v5"
	"strings"
)

const DefaultValue = -1
const DefaultPageStart = 0
const DefaultPageEnd = 100

var GPGKeyTableInit = TableInitStruct{
	TableName:    "gpgKey",
	TableIndexes: []string{"FullFingerPrint", "Names", "Emails"},
}

type GPGKey struct {
	Id                     string
	FullFingerPrint        string
	Names                  []string
	Emails                 []string
	KeyUids                []GPGKeyUid
	KeyBits                int
	AsciiArmoredPublicKey  string
	AsciiArmoredPrivateKey string
}

func AddGPGKey(conn *r.Session, data GPGKey) (string, bool, error) {
	existing, err := r.
		Table(GPGKeyTableInit.TableName).
		GetAllByIndex("FullFingerPrint", data.FullFingerPrint).
		Run(conn)

	if err != nil {
		return "", false, err
	}

	var gpgKey GPGKey

	if existing.Next(gpgKey) {
		// Update
		_, err := r.Table(GPGKeyTableInit.TableName).
			Get(gpgKey.Id).
			Update(data).
			RunWrite(conn)

		if err != nil {
			return "", false, err
		}

		return gpgKey.Id, false, err
	} else {
		// Create
		wr, err := r.Table(GPGKeyTableInit.TableName).
			Insert(data).
			RunWrite(conn)

		if err != nil {
			return "", false, err
		}

		return wr.GeneratedKeys[0], true, err
	}
}

func GetGPGKeyByFingerPrint(conn *r.Session, fingerPrint string) *GPGKey {
	res, err := r.Table(GPGKeyTableInit.TableName).
		Filter(r.Row.Field("FullFingerPrint").Match(fmt.Sprintf("%s$", fingerPrint))).
		Limit(1).
		CoerceTo("array").
		Run(conn)

	if err != nil {
		panic(err)
	}

	var gpgKey GPGKey

	if res.Next(&gpgKey) {
		return &gpgKey
	}

	return nil
}

func SearchGPGKeyByEmail(conn *r.Session, email string, pageStart, pageEnd int) []GPGKey {
	if pageStart < 0 {
		pageStart = DefaultPageStart
	}

	if pageEnd < 0 {
		pageEnd = DefaultPageEnd
	}

	var filterEmailList = func(r r.Term) interface{} {
		return r.Match(email)
	}
	res, err := r.Table(GPGKeyTableInit.TableName).
		Filter(func(r r.Term) interface{} {
			return r.Field("Emails").
				Filter(filterEmailList).
				Count().
				Gt(0)
		}).
		Slice(pageStart, pageEnd).
		CoerceTo("array").
		Run(conn)

	if err != nil {
		panic(err)
	}
	results := make([]GPGKey, 0)
	var gpgKey GPGKey

	for res.Next(&gpgKey) {
		results = append(results, gpgKey)
	}

	return results
}

func SearchGPGKeyByFingerPrint(conn *r.Session, fingerPrint string, pageStart, pageEnd int) []GPGKey {
	if pageStart < 0 {
		pageStart = DefaultPageStart
	}

	if pageEnd < 0 {
		pageEnd = DefaultPageEnd
	}

	res, err := r.Table(GPGKeyTableInit.TableName).
		Filter(r.Row.Field("FullFingerPrint").Match(fmt.Sprintf("%s$", fingerPrint))).
		Slice(pageStart, pageEnd).
		CoerceTo("array").
		Run(conn)

	if err != nil {
		panic(err)
	}
	results := make([]GPGKey, 0)
	var gpgKey GPGKey

	for res.Next(&gpgKey) {
		results = append(results, gpgKey)
	}

	return results
}

func SearchGPGKeyByName(conn *r.Session, name string, pageStart, pageEnd int) []GPGKey {
	if pageStart < 0 {
		pageStart = DefaultPageStart
	}

	if pageEnd < 0 {
		pageEnd = DefaultPageEnd
	}

	var filterNames = func(r r.Term) interface{} {
		return r.Match(name)
	}
	res, err := r.Table(GPGKeyTableInit.TableName).
		Filter(func(r r.Term) interface{} {
			return r.Field("Names").
				Filter(filterNames).
				Count().
				Gt(0)
		}).
		Slice(pageStart, pageEnd).
		CoerceTo("array").
		Run(conn)

	if err != nil {
		panic(err)
	}
	results := make([]GPGKey, 0)
	var gpgKey GPGKey

	for res.Next(&gpgKey) {
		results = append(results, gpgKey)
	}

	return results
}

func AsciiArmored2GPGKey(asciiArmored string) GPGKey {
	var key GPGKey
	reader := bytes.NewBuffer([]byte(asciiArmored))
	z, err := openpgp.ReadArmoredKeyRing(reader)

	if err != nil {
		panic(err)
	}

	if len(z) > 0 {
		entity := z[0]
		pubKey := entity.PrimaryKey
		keyBits, _ := pubKey.BitLength()
		key = GPGKey{
			FullFingerPrint:       strings.ToUpper(hex.EncodeToString(pubKey.Fingerprint[:])),
			AsciiArmoredPublicKey: asciiArmored,
			Emails:                make([]string, 0),
			Names:                 make([]string, 0),
			KeyUids:               make([]GPGKeyUid, 0),
			KeyBits:               int(keyBits),
		}

		for _, v := range entity.Identities {
			z := GPGKeyUid{
				Name:        v.UserId.Name,
				Email:       v.UserId.Email,
				Description: v.UserId.Comment,
			}
			if z.Name != "" || z.Email != "" {
				key.KeyUids = append(key.KeyUids, z)

				if z.Name != "" {
					key.Names = append(key.Names, z.Name)
				}

				if z.Email != "" {
					key.Emails = append(key.Emails, z.Email)
				}
			}
		}

		return key
	}

	panic("Cannot parse GPG Key")
}